package volume_test

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/khanzadimahdi/testproject/application/workload/controlplane/kinds/blocks/blockstest"
	"github.com/khanzadimahdi/testproject/application/workload/controlplane/kinds/requirements/requirementstest"
	"github.com/khanzadimahdi/testproject/application/workload/controlplane/kinds/vm/vmtest"
	"github.com/khanzadimahdi/testproject/domain"
	"github.com/khanzadimahdi/testproject/domain/workload/graph"
	"github.com/khanzadimahdi/testproject/domain/workload/kind"
	networkKind "github.com/khanzadimahdi/testproject/domain/workload/kinds/network"
	stackKind "github.com/khanzadimahdi/testproject/domain/workload/kinds/stack"
	vmKind "github.com/khanzadimahdi/testproject/domain/workload/kinds/vm"
	volumeKind "github.com/khanzadimahdi/testproject/domain/workload/kinds/volume"
)

// wanted is the volume data in the Docker VM vmUUID names, of owner's.
func wanted(vmUUID string) volumeKind.Want {
	return volumeKind.Want{OwnerUUID: "owner", VM: stackKind.VMChoice{UUID: vmUUID}, Spec: volumeKind.Spec{Name: "data"}}
}

// aVolume is the volume data's record, uuid's, in the Docker VM vmUUID
// names, doing state and expected to be expected, and, when it is made, what
// docker has of it.
func aVolume(uuid string, vmUUID string, state kind.State, expected kind.State, made bool) kind.Raw {
	status := volumeKind.Status{Status: kind.Status{State: state, Expected: expected, Since: time.Now(), ObservedAt: time.Now()}}
	if made {
		status.Docker = &volumeKind.Docker{Name: "data", Driver: "local"}
	}

	return blockstest.A[volumeKind.Spec, volumeKind.Status](volumeKind.Name, uuid, vmUUID, volumeKind.Spec{Name: "data"}, status)
}

// state is what the volume kept under uuid is doing, or nothing when none is
// kept.
func state(t *testing.T, w *requirementstest.Workload, uuid string) kind.State {
	t.Helper()

	r, kept := w.Kept(volumeKind.Name, uuid)
	if !kept {
		return ""
	}

	common, err := r.Common()
	require.NoError(t, err)

	return common.State
}

func TestGraph_Requires(t *testing.T) {
	t.Parallel()

	volumes := requirementstest.New().Kinds[volumeKind.Name]

	t.Run("its vm, wanted as it wants it", func(t *testing.T) {
		t.Parallel()

		w := wanted("vm-1")
		w.VM.Name = "builds"

		required, err := volumes.Requires(&w)
		require.NoError(t, err)

		assert.Equal(t, []graph.Requirement{{Kind: vmKind.Name, Name: "vm-1", Spec: vmKind.Want{OwnerUUID: "owner", VM: w.VM}}}, required)
	})

	for name, spec := range map[string]any{
		"what is not a volume's want": networkKind.Want{VM: stackKind.VMChoice{UUID: "vm-1"}, Spec: networkKind.Spec{Name: "data"}},
		"a want in no vm":             wanted(""),
		"a want of no name":           volumeKind.Want{VM: stackKind.VMChoice{UUID: "vm-1"}},
	} {
		t.Run("not "+name, func(t *testing.T) {
			t.Parallel()

			_, err := volumes.Requires(spec)
			assert.ErrorIs(t, err, kind.ErrInvalidPayload)
		})
	}
}

func TestGraph_Exists(t *testing.T) {
	t.Parallel()

	ctx := context.Background()

	// whether it is made, and its record, made or not: none when there is no
	// record of it.
	for name, tt := range map[string]struct {
		volume kind.Raw
		exists bool
		none   bool
	}{
		"one docker has":                                {volume: aVolume("v-1", "vm-1", volumeKind.Present, volumeKind.Present, true), exists: true},
		"one that failed once docker had it":            {volume: aVolume("v-1", "vm-1", volumeKind.Failed, volumeKind.Present, true), exists: true},
		"not one that failed before docker ever had it": {volume: aVolume("v-1", "vm-1", volumeKind.Failed, volumeKind.Present, false)},
		"nor one not made yet":                          {volume: aVolume("v-1", "vm-1", volumeKind.Pending, volumeKind.Present, false)},
		"nor one being made":                            {volume: aVolume("v-1", "vm-1", volumeKind.Creating, volumeKind.Present, false)},
		"nor one its vm has none of":                    {volume: aVolume("v-1", "vm-1", volumeKind.Missing, volumeKind.Present, true)},
		"nor one waiting on its vm":                     {volume: aVolume("v-1", "vm-1", volumeKind.Waiting, volumeKind.Present, true)},
		"nor one being removed":                         {volume: aVolume("v-1", "vm-1", volumeKind.Removing, kind.Deleted, true)},
		"nor one of its name in another vm":             {volume: aVolume("v-1", "vm-2", volumeKind.Present, volumeKind.Present, true), none: true},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			w := requirementstest.New(vmtest.WithVMs(vmtest.Docker("vm-1", "owner"), vmtest.Docker("vm-2", "owner")))
			w.Keep(tt.volume)

			ref, exists, err := w.Kinds[volumeKind.Name].Exists(ctx, wanted("vm-1"))
			require.NoError(t, err)

			assert.Equal(t, tt.exists, exists)

			if tt.none {
				assert.Zero(t, ref, "there is no record of it")
			} else {
				assert.Equal(t, graph.Ref{Kind: volumeKind.Name, UUID: "v-1"}, ref, "its record, made or not")
			}
		})
	}

	t.Run("nor one there is no record of, which is no record", func(t *testing.T) {
		t.Parallel()

		w := requirementstest.New(vmtest.WithVMs(vmtest.Docker("vm-1", "owner")))

		ref, exists, err := w.Kinds[volumeKind.Name].Exists(ctx, wanted("vm-1"))
		require.NoError(t, err)
		assert.False(t, exists)
		assert.Zero(t, ref)
	})
}

func TestGraph_Create(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	inVM := map[graph.NodeID]graph.Ref{graph.ID(vmKind.Name, "vm-1"): {Kind: vmKind.Name, UUID: "vm-1"}}

	t.Run("one there is no record of is admitted into its vm, made there by its node, and is its record", func(t *testing.T) {
		t.Parallel()

		w := requirementstest.New(vmtest.WithVMs(vmtest.Docker("vm-1", "owner")))

		asked := wanted("vm-1")
		asked.Spec.Labels = map[string]string{"backup": "nightly"}

		ref, err := w.Kinds[volumeKind.Name].Create(ctx, asked, inVM)
		require.NoError(t, err)

		r, kept := w.Kept(volumeKind.Name, ref.UUID)
		require.True(t, kept)

		v, err := kind.Decode[volumeKind.Spec, volumeKind.Status](r.Raw)
		require.NoError(t, err)

		assert.Equal(t, volumeKind.Present, v.Status.Status.State, "as its node made it")
		assert.Equal(t, volumeKind.Spec{Name: "data", Labels: map[string]string{"backup": "nightly"}}, v.Spec)
		assert.Equal(t, "owner", v.Metadata.OwnerUUID)
		assert.Equal(t, []kind.Reference{{Kind: "vm", UUID: "vm-1"}}, v.Metadata.Refs, "it lives in its vm")
		assert.Empty(t, v.Metadata.Owners, "and nothing owns it until its graph says so")
		assert.Equal(t, int64(1), v.Metadata.Generation)

		sent := w.Node.Sent()
		require.Len(t, sent, 1)
		assert.Equal(t, volumeKind.ActionCreate, sent[0].Action)
	})

	t.Run("one made already is its record, and nothing is asked", func(t *testing.T) {
		t.Parallel()

		w := requirementstest.New(vmtest.WithVMs(vmtest.Docker("vm-1", "owner")))
		w.Keep(aVolume("v-1", "vm-1", volumeKind.Present, volumeKind.Present, true))

		ref, err := w.Kinds[volumeKind.Name].Create(ctx, wanted("vm-1"), inVM)
		require.NoError(t, err)

		assert.Equal(t, graph.Ref{Kind: volumeKind.Name, UUID: "v-1"}, ref)
		assert.Empty(t, w.Node.Sent())
	})

	for name, volume := range map[string]kind.Raw{
		"one its vm has none of is asked to be made again, empty": aVolume("v-1", "vm-1", volumeKind.Missing, volumeKind.Present, true),
		"one not asked to be made yet is asked":                   aVolume("v-1", "vm-1", volumeKind.Pending, volumeKind.Present, false),
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			w := requirementstest.New(vmtest.WithVMs(vmtest.Docker("vm-1", "owner")))
			w.Keep(volume)

			ref, err := w.Kinds[volumeKind.Name].Create(ctx, wanted("vm-1"), inVM)
			require.NoError(t, err)

			assert.Equal(t, graph.Ref{Kind: volumeKind.Name, UUID: "v-1"}, ref)

			sent := w.Node.Sent()
			require.Len(t, sent, 1)
			assert.Equal(t, volumeKind.ActionCreate, sent[0].Action)
			assert.Equal(t, volumeKind.Present, state(t, w, "v-1"))
		})
	}

	t.Run("one that failed before docker ever had it is not made, saying why, and asked nothing until its vm is seen without it", func(t *testing.T) {
		t.Parallel()

		w := requirementstest.New(vmtest.WithVMs(vmtest.Docker("vm-1", "owner")))
		w.Keep(blockstest.A[volumeKind.Spec, volumeKind.Status](volumeKind.Name, "v-1", "vm-1", volumeKind.Spec{Name: "data"}, volumeKind.Status{
			Status: kind.Status{State: volumeKind.Failed, Expected: volumeKind.Present, Reason: "a volume named data is there already"},
		}))

		_, err := w.Kinds[volumeKind.Name].Create(ctx, wanted("vm-1"), inVM)

		require.Error(t, err)
		assert.Contains(t, err.Error(), "is not made: it is failed: a volume named data is there already")
		assert.Empty(t, w.Node.Sent())
	})

	t.Run("one being made is waited on, and nothing is asked again", func(t *testing.T) {
		t.Parallel()

		w := requirementstest.New(vmtest.WithVMs(vmtest.Docker("vm-1", "owner")))
		w.KeepWaiting(aVolume("v-1", "vm-1", volumeKind.Creating, volumeKind.Present, false), volumeKind.ActionCreate, "create-1")

		waiting, _ := w.Kept(volumeKind.Name, "v-1")
		answered, _ := requirementstest.Carry(requirementstest.CommandOf(waiting))

		said := make(chan error, 1)
		go func() {
			time.Sleep(20 * time.Millisecond)
			said <- w.Node.Say(ctx, answered)
		}()

		ref, err := w.Kinds[volumeKind.Name].Create(ctx, wanted("vm-1"), inVM)
		require.NoError(t, err)
		require.NoError(t, <-said)

		assert.Equal(t, graph.Ref{Kind: volumeKind.Name, UUID: "v-1"}, ref)
		assert.Empty(t, w.Node.Sent())
	})

	t.Run("nothing is asked while its vm does not run, which it waits on", func(t *testing.T) {
		t.Parallel()

		w := requirementstest.New(vmtest.WithVMs(vmtest.In(vmtest.Docker("vm-1", "owner"), func(v *vmKind.VM) { v.Status.State = vmKind.Stopped })))

		_, err := w.Kinds[volumeKind.Name].Create(ctx, wanted("vm-1"), inVM)

		var waiting *graph.Waiting
		require.ErrorAs(t, err, &waiting)
		assert.Equal(t, "its vm is stopped", waiting.Reason)

		assert.Empty(t, w.Node.Sent())
		assert.Zero(t, w.Memory.Len(volumeKind.Name), "and nothing is kept")
	})

	t.Run("a vm that is not there is not waited on", func(t *testing.T) {
		t.Parallel()

		_, err := requirementstest.New().Kinds[volumeKind.Name].Create(ctx, wanted("vm-1"), inVM)

		assert.ErrorIs(t, err, domain.ErrNotExists)
	})
}

func TestGraph_Delete(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	data := graph.Ref{Kind: volumeKind.Name, UUID: "v-1"}

	t.Run("one is removed by force, and is gone once its node removed it", func(t *testing.T) {
		t.Parallel()

		w := requirementstest.New(vmtest.WithVMs(vmtest.Docker("vm-1", "owner")))
		w.Keep(aVolume("v-1", "vm-1", volumeKind.Present, volumeKind.Present, true))

		require.NoError(t, w.Kinds[volumeKind.Name].Delete(ctx, data))

		assert.Empty(t, state(t, w, "v-1"), "it is gone")

		sent := w.Node.Sent()
		require.Len(t, sent, 1)
		assert.Equal(t, volumeKind.ActionDelete, sent[0].Action)
		assert.JSONEq(t, `{"force": true}`, string(sent[0].Payload))
	})

	t.Run("one gone already is removed, and nothing is asked", func(t *testing.T) {
		t.Parallel()

		w := requirementstest.New(vmtest.WithVMs(vmtest.Docker("vm-1", "owner")))

		require.NoError(t, w.Kinds[volumeKind.Name].Delete(ctx, data))
		assert.Empty(t, w.Node.Sent())
	})

	t.Run("one being removed is waited on until it is gone", func(t *testing.T) {
		t.Parallel()

		w := requirementstest.New(vmtest.WithVMs(vmtest.Docker("vm-1", "owner")))
		w.KeepWaiting(aVolume("v-1", "vm-1", volumeKind.Removing, kind.Deleted, true), volumeKind.ActionDelete, "delete-1")

		removing, _ := w.Kept(volumeKind.Name, "v-1")
		answered, _ := requirementstest.Carry(requirementstest.CommandOf(removing))

		said := make(chan error, 1)
		go func() {
			time.Sleep(20 * time.Millisecond)
			said <- w.Node.Say(ctx, answered)
		}()

		require.NoError(t, w.Kinds[volumeKind.Name].Delete(ctx, data))
		require.NoError(t, <-said)

		assert.Empty(t, state(t, w, "v-1"))
		assert.Empty(t, w.Node.Sent())
	})

	t.Run("nothing is asked while its vm does not run, which it waits on", func(t *testing.T) {
		t.Parallel()

		w := requirementstest.New(vmtest.WithVMs(vmtest.In(vmtest.Docker("vm-1", "owner"), func(v *vmKind.VM) { v.Status.State = vmKind.Stopped })))
		w.Keep(aVolume("v-1", "vm-1", volumeKind.Waiting, volumeKind.Present, true))

		err := w.Kinds[volumeKind.Name].Delete(ctx, data)

		var waiting *graph.Waiting
		require.ErrorAs(t, err, &waiting)
		assert.Equal(t, "its vm is stopped", waiting.Reason)

		assert.Equal(t, volumeKind.Waiting, state(t, w, "v-1"))
		assert.Empty(t, w.Node.Sent())
	})
}
