package network_test

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
	containerKind "github.com/khanzadimahdi/testproject/domain/workload/kinds/container"
	networkKind "github.com/khanzadimahdi/testproject/domain/workload/kinds/network"
	stackKind "github.com/khanzadimahdi/testproject/domain/workload/kinds/stack"
	vmKind "github.com/khanzadimahdi/testproject/domain/workload/kinds/vm"
)

// wanted is the network backend in the Docker VM vmUUID names, of owner's.
func wanted(vmUUID string) networkKind.Want {
	return networkKind.Want{OwnerUUID: "owner", VM: stackKind.VMChoice{UUID: vmUUID}, Spec: networkKind.Spec{Name: "backend"}}
}

// aNetwork is the network backend's record, uuid's, in the Docker VM vmUUID
// names, doing state and expected to be expected, and, when it is made, what
// docker has of it.
func aNetwork(uuid string, vmUUID string, state kind.State, expected kind.State, made bool) kind.Raw {
	status := networkKind.Status{Status: kind.Status{State: state, Expected: expected, Since: time.Now(), ObservedAt: time.Now()}}
	if made {
		status.Docker = &networkKind.Docker{ID: "n-" + uuid, Name: "backend", Driver: "bridge", Containers: []string{}}
	}

	return blockstest.A[networkKind.Spec, networkKind.Status](networkKind.Name, uuid, vmUUID, networkKind.Spec{Name: "backend"}, status)
}

// state is what the network kept under uuid is doing, or nothing when none
// is kept.
func state(t *testing.T, w *requirementstest.Workload, uuid string) kind.State {
	t.Helper()

	r, kept := w.Kept(networkKind.Name, uuid)
	if !kept {
		return ""
	}

	common, err := r.Common()
	require.NoError(t, err)

	return common.State
}

func TestGraph_Requires(t *testing.T) {
	t.Parallel()

	networks := requirementstest.New().Kinds[networkKind.Name]

	t.Run("its vm, wanted as it wants it", func(t *testing.T) {
		t.Parallel()

		w := wanted("vm-1")
		w.VM.Name = "builds"

		required, err := networks.Requires(w)
		require.NoError(t, err)

		assert.Equal(t, []graph.Requirement{{Kind: vmKind.Name, Name: "vm-1", Spec: vmKind.Want{OwnerUUID: "owner", VM: w.VM}}}, required)
	})

	for name, spec := range map[string]any{
		"what is not a network's want": containerKind.Want{VM: stackKind.VMChoice{UUID: "vm-1"}},
		"a want in no vm":              wanted(""),
		"a want of no name":            networkKind.Want{VM: stackKind.VMChoice{UUID: "vm-1"}, Spec: networkKind.Spec{Name: " "}},
	} {
		t.Run("not "+name, func(t *testing.T) {
			t.Parallel()

			_, err := networks.Requires(spec)
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
		network kind.Raw
		exists  bool
		none    bool
	}{
		"one docker has":                                {network: aNetwork("n-1", "vm-1", networkKind.Present, networkKind.Present, true), exists: true},
		"one that failed once docker had it":            {network: aNetwork("n-1", "vm-1", networkKind.Failed, networkKind.Present, true), exists: true},
		"not one that failed before docker ever had it": {network: aNetwork("n-1", "vm-1", networkKind.Failed, networkKind.Present, false)},
		"nor one not made yet":                          {network: aNetwork("n-1", "vm-1", networkKind.Pending, networkKind.Present, false)},
		"nor one being made":                            {network: aNetwork("n-1", "vm-1", networkKind.Creating, networkKind.Present, false)},
		"nor one its vm has none of":                    {network: aNetwork("n-1", "vm-1", networkKind.Missing, networkKind.Present, true)},
		"nor one waiting on its vm":                     {network: aNetwork("n-1", "vm-1", networkKind.Waiting, networkKind.Present, true)},
		"nor one being removed":                         {network: aNetwork("n-1", "vm-1", networkKind.Removing, kind.Deleted, true)},
		"nor one of its name in another vm":             {network: aNetwork("n-1", "vm-2", networkKind.Present, networkKind.Present, true), none: true},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			w := requirementstest.New(vmtest.WithVMs(vmtest.Docker("vm-1", "owner"), vmtest.Docker("vm-2", "owner")))
			w.Keep(tt.network)

			ref, exists, err := w.Kinds[networkKind.Name].Exists(ctx, wanted("vm-1"))
			require.NoError(t, err)

			assert.Equal(t, tt.exists, exists)

			if tt.none {
				assert.Zero(t, ref, "there is no record of it")
			} else {
				assert.Equal(t, graph.Ref{Kind: networkKind.Name, UUID: "n-1"}, ref, "its record, made or not")
			}
		})
	}

	t.Run("nor one there is no record of, which is no record", func(t *testing.T) {
		t.Parallel()

		w := requirementstest.New(vmtest.WithVMs(vmtest.Docker("vm-1", "owner")))

		ref, exists, err := w.Kinds[networkKind.Name].Exists(ctx, wanted("vm-1"))
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
		asked.Labels = map[string]string{"stack": "shop"}
		asked.Spec.Internal = true

		ref, err := w.Kinds[networkKind.Name].Create(ctx, asked, inVM)
		require.NoError(t, err)

		r, kept := w.Kept(networkKind.Name, ref.UUID)
		require.True(t, kept)

		n, err := kind.Decode[networkKind.Spec, networkKind.Status](r.Raw)
		require.NoError(t, err)

		assert.Equal(t, networkKind.Present, n.Status.Status.State, "as its node made it")
		assert.Equal(t, networkKind.Spec{Name: "backend", Internal: true}, n.Spec)
		assert.Equal(t, "owner", n.Metadata.OwnerUUID)
		assert.Equal(t, map[string]string{"stack": "shop"}, n.Metadata.Labels)
		assert.Equal(t, []kind.Reference{{Kind: "vm", UUID: "vm-1"}}, n.Metadata.Refs, "it lives in its vm")
		assert.Empty(t, n.Metadata.Owners, "and nothing owns it until its graph says so")
		assert.Equal(t, int64(1), n.Metadata.Generation)

		sent := w.Node.Sent()
		require.Len(t, sent, 1)
		assert.Equal(t, networkKind.ActionCreate, sent[0].Action)
	})

	t.Run("one made already is its record, and nothing is asked", func(t *testing.T) {
		t.Parallel()

		w := requirementstest.New(vmtest.WithVMs(vmtest.Docker("vm-1", "owner")))
		w.Keep(aNetwork("n-1", "vm-1", networkKind.Present, networkKind.Present, true))

		ref, err := w.Kinds[networkKind.Name].Create(ctx, wanted("vm-1"), inVM)
		require.NoError(t, err)

		assert.Equal(t, graph.Ref{Kind: networkKind.Name, UUID: "n-1"}, ref)
		assert.Empty(t, w.Node.Sent())
	})

	for name, network := range map[string]kind.Raw{
		"one its vm has none of is asked to be made again": aNetwork("n-1", "vm-1", networkKind.Missing, networkKind.Present, true),
		"one not asked to be made yet is asked":            aNetwork("n-1", "vm-1", networkKind.Pending, networkKind.Present, false),
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			w := requirementstest.New(vmtest.WithVMs(vmtest.Docker("vm-1", "owner")))
			w.Keep(network)

			ref, err := w.Kinds[networkKind.Name].Create(ctx, wanted("vm-1"), inVM)
			require.NoError(t, err)

			assert.Equal(t, graph.Ref{Kind: networkKind.Name, UUID: "n-1"}, ref)

			sent := w.Node.Sent()
			require.Len(t, sent, 1)
			assert.Equal(t, networkKind.ActionCreate, sent[0].Action)
			assert.Equal(t, networkKind.Present, state(t, w, "n-1"))
		})
	}

	t.Run("one that failed before docker ever had it is not made, saying why, and asked nothing until its vm is seen without it", func(t *testing.T) {
		t.Parallel()

		w := requirementstest.New(vmtest.WithVMs(vmtest.Docker("vm-1", "owner")))
		w.Keep(blockstest.A[networkKind.Spec, networkKind.Status](networkKind.Name, "n-1", "vm-1", networkKind.Spec{Name: "backend"}, networkKind.Status{
			Status: kind.Status{State: networkKind.Failed, Expected: networkKind.Present, Reason: "network with name backend already exists"},
		}))

		_, err := w.Kinds[networkKind.Name].Create(ctx, wanted("vm-1"), inVM)

		require.Error(t, err)
		assert.Contains(t, err.Error(), "is not made: it is failed: network with name backend already exists")
		assert.Empty(t, w.Node.Sent())
	})

	t.Run("one being made is waited on, and nothing is asked again", func(t *testing.T) {
		t.Parallel()

		w := requirementstest.New(vmtest.WithVMs(vmtest.Docker("vm-1", "owner")))
		w.KeepWaiting(aNetwork("n-1", "vm-1", networkKind.Creating, networkKind.Present, false), networkKind.ActionCreate, "create-1")

		waiting, _ := w.Kept(networkKind.Name, "n-1")
		answered, _ := requirementstest.Carry(requirementstest.CommandOf(waiting))

		said := make(chan error, 1)
		go func() {
			time.Sleep(20 * time.Millisecond)
			said <- w.Node.Say(ctx, answered)
		}()

		ref, err := w.Kinds[networkKind.Name].Create(ctx, wanted("vm-1"), inVM)
		require.NoError(t, err)
		require.NoError(t, <-said)

		assert.Equal(t, graph.Ref{Kind: networkKind.Name, UUID: "n-1"}, ref)
		assert.Empty(t, w.Node.Sent())
	})

	t.Run("nothing is asked while its vm does not run, which it waits on", func(t *testing.T) {
		t.Parallel()

		w := requirementstest.New(vmtest.WithVMs(vmtest.In(vmtest.Docker("vm-1", "owner"), func(v *vmKind.VM) { v.Status.State = vmKind.Stopped })))

		_, err := w.Kinds[networkKind.Name].Create(ctx, wanted("vm-1"), inVM)

		var waiting *graph.Waiting
		require.ErrorAs(t, err, &waiting)
		assert.Equal(t, "its vm is stopped", waiting.Reason)

		assert.Empty(t, w.Node.Sent())
		assert.Zero(t, w.Memory.Len(networkKind.Name), "and nothing is kept")
	})

	t.Run("one on its way out is waited on", func(t *testing.T) {
		t.Parallel()

		w := requirementstest.New(vmtest.WithVMs(vmtest.Docker("vm-1", "owner")))
		w.Keep(aNetwork("n-1", "vm-1", networkKind.Removing, kind.Deleted, true))

		_, err := w.Kinds[networkKind.Name].Create(ctx, wanted("vm-1"), inVM)

		var waiting *graph.Waiting
		require.ErrorAs(t, err, &waiting)
		assert.Equal(t, "it is being removed", waiting.Reason)
	})

	t.Run("a vm that is not there is not waited on", func(t *testing.T) {
		t.Parallel()

		_, err := requirementstest.New().Kinds[networkKind.Name].Create(ctx, wanted("vm-1"), inVM)

		assert.ErrorIs(t, err, domain.ErrNotExists)
	})
}

func TestGraph_Delete(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	backend := graph.Ref{Kind: networkKind.Name, UUID: "n-1"}

	t.Run("one is removed, and is gone once its node removed it", func(t *testing.T) {
		t.Parallel()

		w := requirementstest.New(vmtest.WithVMs(vmtest.Docker("vm-1", "owner")))
		w.Keep(aNetwork("n-1", "vm-1", networkKind.Present, networkKind.Present, true))

		require.NoError(t, w.Kinds[networkKind.Name].Delete(ctx, backend))

		assert.Empty(t, state(t, w, "n-1"), "it is gone")

		sent := w.Node.Sent()
		require.Len(t, sent, 1)
		assert.Equal(t, networkKind.ActionDelete, sent[0].Action)
		assert.Empty(t, sent[0].Payload, "asked with nothing")
	})

	t.Run("one gone already is removed, and nothing is asked", func(t *testing.T) {
		t.Parallel()

		w := requirementstest.New(vmtest.WithVMs(vmtest.Docker("vm-1", "owner")))

		require.NoError(t, w.Kinds[networkKind.Name].Delete(ctx, backend))
		assert.Empty(t, w.Node.Sent())
	})

	t.Run("one being removed is waited on until it is gone", func(t *testing.T) {
		t.Parallel()

		w := requirementstest.New(vmtest.WithVMs(vmtest.Docker("vm-1", "owner")))
		w.KeepWaiting(aNetwork("n-1", "vm-1", networkKind.Removing, kind.Deleted, true), networkKind.ActionDelete, "delete-1")

		removing, _ := w.Kept(networkKind.Name, "n-1")
		answered, _ := requirementstest.Carry(requirementstest.CommandOf(removing))

		said := make(chan error, 1)
		go func() {
			time.Sleep(20 * time.Millisecond)
			said <- w.Node.Say(ctx, answered)
		}()

		require.NoError(t, w.Kinds[networkKind.Name].Delete(ctx, backend))
		require.NoError(t, <-said)

		assert.Empty(t, state(t, w, "n-1"))
		assert.Empty(t, w.Node.Sent())
	})

	t.Run("nothing is asked while its vm does not run, which it waits on", func(t *testing.T) {
		t.Parallel()

		w := requirementstest.New(vmtest.WithVMs(vmtest.In(vmtest.Docker("vm-1", "owner"), func(v *vmKind.VM) { v.Status.State = vmKind.Stopped })))
		w.Keep(aNetwork("n-1", "vm-1", networkKind.Waiting, networkKind.Present, true))

		err := w.Kinds[networkKind.Name].Delete(ctx, backend)

		var waiting *graph.Waiting
		require.ErrorAs(t, err, &waiting)
		assert.Equal(t, "its vm is stopped", waiting.Reason)

		assert.Equal(t, networkKind.Waiting, state(t, w, "n-1"))
		assert.Empty(t, w.Node.Sent())
	})

	t.Run("one a container is still on is not removed, as docker says", func(t *testing.T) {
		t.Parallel()

		w := requirementstest.New(vmtest.WithVMs(vmtest.Docker("vm-1", "owner")))
		w.Keep(aNetwork("n-1", "vm-1", networkKind.Present, networkKind.Present, true))
		w.Node.Answer(func(command kind.ActOnResource) (kind.ResourceActedOn, bool) {
			result := requirementstest.Fail(command, "error while removing network: network backend has active endpoints")
			result.Refused = true
			result.Status = []byte(`{"state": "present", "failure": {"code": "invalid", "message": "error while removing network: network backend has active endpoints"}}`)

			return result, true
		})

		err := w.Kinds[networkKind.Name].Delete(ctx, backend)

		require.Error(t, err)
		assert.Contains(t, err.Error(), "is not removed: it is present: error while removing network: network backend has active endpoints")
		assert.Equal(t, networkKind.Present, state(t, w, "n-1"), "it is left as it was")
	})
}
