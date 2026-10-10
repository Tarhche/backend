package vm_test

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/khanzadimahdi/testproject/application/workload/controlplane/kinds/blocks/blockstest"
	"github.com/khanzadimahdi/testproject/application/workload/controlplane/kinds/requirements/requirementstest"
	"github.com/khanzadimahdi/testproject/application/workload/controlplane/kinds/vm/vmtest"
	"github.com/khanzadimahdi/testproject/domain/workload/graph"
	"github.com/khanzadimahdi/testproject/domain/workload/kind"
	containerKind "github.com/khanzadimahdi/testproject/domain/workload/kinds/container"
	stackKind "github.com/khanzadimahdi/testproject/domain/workload/kinds/stack"
	vmKind "github.com/khanzadimahdi/testproject/domain/workload/kinds/vm"
	"github.com/khanzadimahdi/testproject/domain/workload/port"
	"github.com/khanzadimahdi/testproject/domain/workload/vm"
)

// wantedVM is the Docker VM uuid names, of owner's, wanted with nothing
// beyond the defaults.
func wantedVM(uuid string) vmKind.Want {
	return vmKind.Want{OwnerUUID: "owner", VM: stackKind.VMChoice{UUID: uuid}}
}

// encoded is a VM as it is kept.
func encoded(t *testing.T, v vmKind.VM) kind.Raw {
	t.Helper()

	raw, err := kind.Encode(v)
	require.NoError(t, err)

	return raw
}

func TestGraph_Requires(t *testing.T) {
	t.Parallel()

	vms := requirementstest.New().Kinds[vmKind.Name]

	required, err := vms.Requires(wantedVM("vm-1"))
	require.NoError(t, err)
	assert.Empty(t, required, "a vm needs nothing a graph makes")

	for name, spec := range map[string]any{
		"what is not a vm's want": containerKind.Want{VM: stackKind.VMChoice{UUID: "vm-1"}},
		"a vm named by no uuid":   wantedVM(""),
	} {
		_, err := vms.Requires(spec)
		assert.ErrorIs(t, err, kind.ErrInvalidPayload, name)
	}
}

func TestGraph_Exists(t *testing.T) {
	t.Parallel()

	ctx := context.Background()

	for name, tt := range map[string]struct {
		vm     vmKind.VM
		exists bool
	}{
		"one that runs":                          {vm: vmtest.Docker("vm-1", "owner"), exists: true},
		"one stopped, which its own loop starts": {vm: vmtest.In(vmtest.Docker("vm-1", "owner"), func(v *vmKind.VM) { v.Status.State = vmKind.Stopped }), exists: true},
		"one that failed":                        {vm: vmtest.In(vmtest.Docker("vm-1", "owner"), func(v *vmKind.VM) { v.Status.State = vmKind.Failed }), exists: true},
		"one not made yet":                       {vm: vmtest.In(vmtest.Docker("vm-1", "owner"), func(v *vmKind.VM) { v.Status.State = vmKind.Created; v.Status.Applied = nil }), exists: true},
		"not one being deleted": {vm: vmtest.In(vmtest.Docker("vm-1", "owner"), func(v *vmKind.VM) {
			v.Status.State = vmKind.Deleting
			v.Status.Expected = vmKind.Deleted
		})},
		"nor one that runs and is to be deleted": {vm: vmtest.In(vmtest.Docker("vm-1", "owner"), func(v *vmKind.VM) { v.Status.Expected = vmKind.Deleted })},
		"nor another":                            {vm: vmtest.Docker("vm-2", "owner")},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			w := requirementstest.New(vmtest.WithVMs(tt.vm))

			ref, exists, err := w.Kinds[vmKind.Name].Exists(ctx, wantedVM("vm-1"))
			require.NoError(t, err)

			assert.Equal(t, tt.exists, exists)

			if tt.exists {
				assert.Equal(t, graph.Ref{Kind: vmKind.Name, UUID: "vm-1"}, ref)
			}
		})
	}
}

func TestGraph_Create(t *testing.T) {
	t.Parallel()

	ctx := context.Background()

	t.Run("one that is not there is made under the uuid it is named by, with the docker defaults and what it is wanted with beyond them, and is it once its node made it", func(t *testing.T) {
		t.Parallel()

		w := requirementstest.New()

		ref, err := w.Kinds[vmKind.Name].Create(ctx, vmKind.Want{
			OwnerUUID: "owner",
			VM:        stackKind.VMChoice{UUID: "vm-made", Name: "builds", Resources: &stackKind.Resources{Memory: 4 * vmtest.GiB}, Ports: []port.Port{}},
		}, nil)
		require.NoError(t, err)

		assert.Equal(t, graph.Ref{Kind: vmKind.Name, UUID: "vm-made"}, ref)

		made, kept := w.Stored("vm-made")
		require.True(t, kept)

		assert.Equal(t, vmKind.Running, made.Status.State, "as its node made it")
		assert.Equal(t, "owner", made.Metadata.OwnerUUID)
		assert.Equal(t, "builds", made.Metadata.Name)
		assert.True(t, vmKind.DockerVM(made, vmtest.Images.Docker), "a docker vm")
		assert.Equal(t, vmKind.Resources{CPUs: 2, Memory: 4 * vmtest.GiB, Disk: 20 * vmtest.GiB}, made.Spec.Resources, "the defaults, but for what it is wanted with")
		assert.Equal(t, []port.Port{}, made.Spec.Ports, "ports wanted empty are none")
		assert.Equal(t, vmtest.DockerDefaults.Network, made.Spec.Network)
		assert.Equal(t, vmtest.Node, made.Metadata.Node, "placed as any vm is")

		sent := w.Node.Sent()
		require.Len(t, sent, 1)
		assert.Equal(t, vmKind.ActionCreate, sent[0].Action)
		assert.Equal(t, "vm-made", sent[0].UUID)
	})

	t.Run("and what needs one vm makes one, however often it is asked", func(t *testing.T) {
		t.Parallel()

		w := requirementstest.New()

		first, err := w.Kinds[vmKind.Name].Create(ctx, wantedVM("vm-made"), nil)
		require.NoError(t, err)

		again, err := w.Kinds[vmKind.Name].Create(ctx, wantedVM("vm-made"), nil)
		require.NoError(t, err)

		assert.Equal(t, first, again)
		assert.Equal(t, 1, w.Memory.Len(vmKind.Name))
		assert.Len(t, w.Node.Sent(), 1)
	})

	for name, v := range map[string]vmKind.VM{
		"one there already is it, and nothing is made":               vmtest.Docker("vm-1", "owner"),
		"whatever it is doing: its own loop starts one that stopped": vmtest.In(vmtest.Docker("vm-1", "owner"), func(v *vmKind.VM) { v.Status.State = vmKind.Stopped }),
		"and asks again for one that failed":                         vmtest.In(vmtest.Docker("vm-1", "owner"), func(v *vmKind.VM) { v.Status.State = vmKind.Failed }),
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			w := requirementstest.New(vmtest.WithVMs(v))

			ref, err := w.Kinds[vmKind.Name].Create(ctx, wantedVM("vm-1"), nil)
			require.NoError(t, err)

			assert.Equal(t, graph.Ref{Kind: vmKind.Name, UUID: "vm-1"}, ref)
			assert.Empty(t, w.Node.Sent())
			assert.Equal(t, 1, w.Memory.Len(vmKind.Name))
		})
	}

	t.Run("one being made is waited on, and nothing is asked again", func(t *testing.T) {
		t.Parallel()

		w := requirementstest.New()
		w.KeepWaiting(encoded(t, vmtest.In(vmtest.Docker("vm-1", "owner"), func(v *vmKind.VM) {
			v.Status.State = vmKind.Scheduled
			v.Status.Applied = nil
		})), vmKind.ActionCreate, "create-1")

		waiting, _ := w.Kept(vmKind.Name, "vm-1")
		answered, _ := requirementstest.Carry(requirementstest.CommandOf(waiting))

		said := make(chan error, 1)
		go func() {
			time.Sleep(20 * time.Millisecond)
			said <- w.Node.Say(ctx, answered)
		}()

		ref, err := w.Kinds[vmKind.Name].Create(ctx, wantedVM("vm-1"), nil)
		require.NoError(t, err)
		require.NoError(t, <-said)

		assert.Equal(t, graph.Ref{Kind: vmKind.Name, UUID: "vm-1"}, ref)
		assert.Empty(t, w.Node.Sent())

		made, _ := w.Stored("vm-1")
		assert.Equal(t, vmKind.Running, made.Status.State)
	})

	t.Run("one on its way out is waited on, until it is gone and can be made again under its uuid", func(t *testing.T) {
		t.Parallel()

		w := requirementstest.New(vmtest.WithVMs(vmtest.In(vmtest.Docker("vm-1", "owner"), func(v *vmKind.VM) {
			v.Status.State = vmKind.Deleting
			v.Status.Expected = vmKind.Deleted
		})))

		_, err := w.Kinds[vmKind.Name].Create(ctx, wantedVM("vm-1"), nil)

		var waiting *graph.Waiting
		require.ErrorAs(t, err, &waiting)
		assert.Equal(t, "it is being deleted", waiting.Reason)
		assert.Empty(t, w.Node.Sent())
	})

	t.Run("one its node could not make is not made, saying why, and is kept failed for its own loop to ask for again", func(t *testing.T) {
		t.Parallel()

		w := requirementstest.New()
		w.Node.Answer(func(command kind.ActOnResource) (kind.ResourceActedOn, bool) {
			return requirementstest.Fail(command, "the vmhost is not answering"), true
		})

		_, err := w.Kinds[vmKind.Name].Create(ctx, wantedVM("vm-made"), nil)

		require.Error(t, err)
		assert.Contains(t, err.Error(), "is not made: it is failed: the vmhost is not answering")

		made, kept := w.Stored("vm-made")
		require.True(t, kept)
		assert.Equal(t, vmKind.Failed, made.Status.State)
		assert.Equal(t, vmKind.Running, made.Status.Expected)
	})

	t.Run("one no node has room for is refused, and nothing is kept", func(t *testing.T) {
		t.Parallel()

		full := vmtest.Alive(vmtest.Node)
		full.Capacity.Allocated = vm.Resources{Memory: 64 * vmtest.GiB}

		w := requirementstest.New(vmtest.WithNodes(full))

		_, err := w.Kinds[vmKind.Name].Create(ctx, wantedVM("vm-made"), nil)

		require.Error(t, err)
		assert.Contains(t, err.Error(), "vm: no_capacity")
		assert.Zero(t, w.Memory.Len(vmKind.Name))
	})

	t.Run("what its admission refuses is refused, saying why", func(t *testing.T) {
		t.Parallel()

		w := requirementstest.New()

		want := wantedVM("vm-made")
		want.VM.Resources = &stackKind.Resources{CPUs: 64}

		_, err := w.Kinds[vmKind.Name].Create(ctx, want, nil)

		require.Error(t, err)
		assert.Contains(t, err.Error(), "vm.resources.cpus: too_large")
		assert.Zero(t, w.Memory.Len(vmKind.Name))
	})
}

func TestGraph_Delete(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	box := graph.Ref{Kind: vmKind.Name, UUID: "vm-1"}

	t.Run("one is removed, what lives in it with it, and is gone once its node removed it", func(t *testing.T) {
		t.Parallel()

		w := requirementstest.New(vmtest.WithVMs(vmtest.Docker("vm-1", "owner")))
		w.Keep(blockstest.AContainer("c-1", "vm-1", containerKind.Running, containerKind.Running))

		require.NoError(t, w.Kinds[vmKind.Name].Delete(ctx, box))

		_, kept := w.Stored("vm-1")
		assert.False(t, kept)

		_, kept = w.Kept(containerKind.Name, "c-1")
		assert.False(t, kept, "as a vm's delete always takes it")

		sent := w.Node.Sent()
		require.Len(t, sent, 1)
		assert.Equal(t, vmKind.ActionDelete, sent[0].Action)
	})

	t.Run("one gone already is removed, and nothing is asked", func(t *testing.T) {
		t.Parallel()

		w := requirementstest.New()

		require.NoError(t, w.Kinds[vmKind.Name].Delete(ctx, box))
		assert.Empty(t, w.Node.Sent())
	})

	t.Run("one being deleted is waited on until it is gone", func(t *testing.T) {
		t.Parallel()

		w := requirementstest.New()
		w.KeepWaiting(encoded(t, vmtest.In(vmtest.Docker("vm-1", "owner"), func(v *vmKind.VM) {
			v.Status.State = vmKind.Deleting
			v.Status.Expected = vmKind.Deleted
		})), vmKind.ActionDelete, "delete-1")

		deleting, _ := w.Kept(vmKind.Name, "vm-1")
		answered, _ := requirementstest.Carry(requirementstest.CommandOf(deleting))

		said := make(chan error, 1)
		go func() {
			time.Sleep(20 * time.Millisecond)
			said <- w.Node.Say(ctx, answered)
		}()

		require.NoError(t, w.Kinds[vmKind.Name].Delete(ctx, box))
		require.NoError(t, <-said)

		_, kept := w.Stored("vm-1")
		assert.False(t, kept)
		assert.Empty(t, w.Node.Sent())
	})

	t.Run("what is not a vm is not a vm's to remove", func(t *testing.T) {
		t.Parallel()

		err := requirementstest.New().Kinds[vmKind.Name].Delete(ctx, graph.Ref{Kind: containerKind.Name, UUID: "vm-1"})
		assert.ErrorIs(t, err, kind.ErrUnknownKind)
	})
}
