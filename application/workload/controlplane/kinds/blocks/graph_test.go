package blocks_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/khanzadimahdi/testproject/application/workload/controlplane/kinds/blocks"
	"github.com/khanzadimahdi/testproject/application/workload/controlplane/kinds/blocks/blockstest"
	"github.com/khanzadimahdi/testproject/application/workload/controlplane/kinds/vm/vmtest"
	"github.com/khanzadimahdi/testproject/domain"
	"github.com/khanzadimahdi/testproject/domain/workload/graph"
	"github.com/khanzadimahdi/testproject/domain/workload/kind"
	containerKind "github.com/khanzadimahdi/testproject/domain/workload/kinds/container"
	stackKind "github.com/khanzadimahdi/testproject/domain/workload/kinds/stack"
	vmKind "github.com/khanzadimahdi/testproject/domain/workload/kinds/vm"
	"github.com/khanzadimahdi/testproject/domain/workload/resource"
)

func TestBlocks_Recorded(t *testing.T) {
	t.Parallel()

	ctx := context.Background()

	w := blockstest.New(vmtest.WithVMs(vmtest.Docker("vm-1", "owner"), vmtest.Docker("vm-2", "owner")))
	w.Keep(blockstest.AContainer("old-web", "vm-1", containerKind.Removing, kind.Deleted))
	w.Keep(blockstest.AContainer("web", "vm-1", containerKind.Running, containerKind.Running))
	w.Keep(blockstest.AContainer("gone-db", "vm-1", containerKind.Removing, kind.Deleted, func(c *containerKind.Container) { c.Spec.Name = "db" }))
	w.Keep(blockstest.AContainer("other-web", "vm-2", containerKind.Running, containerKind.Running))

	r, found, err := w.Containers.Recorded(ctx, "vm-1", "web")
	require.NoError(t, err)
	require.True(t, found)
	assert.Equal(t, "web", r.Metadata.UUID, "of two asked for under its name in its vm, the one that is not on its way out")

	r, found, err = w.Containers.Recorded(ctx, "vm-1", "db")
	require.NoError(t, err)
	require.True(t, found)
	assert.Equal(t, "gone-db", r.Metadata.UUID, "and one on its way out when there is no other")

	_, found, err = w.Containers.Recorded(ctx, "vm-1", "api")
	require.NoError(t, err)
	assert.False(t, found, "and none of a name nothing was asked for under")

	r, found, err = w.Containers.Recorded(ctx, "vm-2", "web")
	require.NoError(t, err)
	require.True(t, found)
	assert.Equal(t, "other-web", r.Metadata.UUID, "in the vm asked about alone")
}

func TestBlocks_Ready(t *testing.T) {
	t.Parallel()

	ctx := context.Background()

	w := blockstest.New(vmtest.WithVMs(
		vmtest.Docker("running", "owner"),
		vmtest.In(vmtest.Docker("stopped", "owner"), func(v *vmKind.VM) { v.Status.State = vmKind.Stopped }),
		vmtest.In(vmtest.Docker("restarting", "owner"), func(v *vmKind.VM) { v.Status.State = vmKind.Restarting }),
		vmtest.In(vmtest.Docker("deleting", "owner"), func(v *vmKind.VM) { v.Status.Expected = vmKind.Deleted }),
	))

	require.NoError(t, w.Containers.Ready(ctx, "running"), "what lives in a vm that runs can be made or removed now")

	for vmUUID, reason := range map[string]string{
		"stopped":    "its vm is stopped",
		"restarting": "its vm is restarting",
		"deleting":   "its vm is being deleted",
	} {
		var waiting *graph.Waiting
		require.ErrorAs(t, w.Containers.Ready(ctx, vmUUID), &waiting, vmUUID)
		assert.Equal(t, reason, waiting.Reason)
	}

	assert.ErrorIs(t, w.Containers.Ready(ctx, "nowhere"), domain.ErrNotExists, "and one that is not there is not")
}

func TestBlocks_Placed(t *testing.T) {
	t.Parallel()

	ctx := context.Background()

	elsewhere := func(v *vmKind.VM) { v.Metadata.Node = "workload-orchestrator-02" }

	for name, tt := range map[string]struct {
		vm      vmKind.VM
		node    string
		placed  string
		written bool
	}{
		"one on no node, admitted before its vm was made, takes its vm's node": {
			vm: vmtest.Docker("vm-1", "owner"), placed: vmtest.Node, written: true,
		},
		"and one on another, its vm made again elsewhere, takes it too": {
			vm: vmtest.In(vmtest.Docker("vm-1", "owner"), elsewhere), node: vmtest.Node, placed: "workload-orchestrator-02", written: true,
		},
		"one on its vm's node is left as it is": {
			vm: vmtest.Docker("vm-1", "owner"), node: vmtest.Node, placed: vmtest.Node,
		},
		"and so is one whose vm is on no node yet": {
			vm: vmtest.In(vmtest.Docker("vm-1", "owner"), func(v *vmKind.VM) { v.Metadata.Node = "" }), placed: "",
		},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			w := blockstest.New(vmtest.WithVMs(tt.vm))
			w.Keep(blockstest.AContainer("web", "vm-1", kind.Missing, containerKind.Running, func(c *containerKind.Container) {
				c.Metadata.Node = tt.node
			}))

			r, _ := w.Kept(containerKind.Name, "web")

			placed, err := w.Containers.Placed(ctx, r, "vm-1")
			require.NoError(t, err)

			assert.Equal(t, tt.placed, placed.Metadata.Node)

			kept, _ := w.Kept(containerKind.Name, "web")
			assert.Equal(t, tt.placed, kept.Metadata.Node, "as it is kept")
			assert.Equal(t, tt.written, kept.Version > r.Version, "written only when it moved")
			assert.Equal(t, kept.Version, placed.Version)
		})
	}

	t.Run("one whose vm is not there is left as it is", func(t *testing.T) {
		t.Parallel()

		w := blockstest.New()
		w.Keep(blockstest.AContainer("web", "vm-gone", kind.Missing, containerKind.Running, func(c *containerKind.Container) {
			c.Metadata.Node = ""
		}))

		r, _ := w.Kept(containerKind.Name, "web")

		placed, err := w.Containers.Placed(ctx, r, "vm-gone")
		require.NoError(t, err)
		assert.Equal(t, r, placed)
	})

	t.Run("and one written in the meantime is not written over", func(t *testing.T) {
		t.Parallel()

		w := blockstest.New(vmtest.WithVMs(vmtest.Docker("vm-1", "owner")))
		w.Keep(blockstest.AContainer("web", "vm-1", kind.Missing, containerKind.Running, func(c *containerKind.Container) {
			c.Metadata.Node = ""
		}))

		stale, _ := w.Kept(containerKind.Name, "web")

		fresh := stale.Clone()
		fresh.Attempts = 1
		_, err := w.Memory.Update(ctx, fresh)
		require.NoError(t, err)

		_, err = w.Containers.Placed(ctx, stale, "vm-1")
		assert.ErrorIs(t, err, resource.ErrConflict)
	})
}

func TestParentOf(t *testing.T) {
	t.Parallel()

	choice := stackKind.VMChoice{UUID: "vm-1", Name: "builds"}

	assert.Equal(t, "vm-1", blocks.ParentOf(choice, map[graph.NodeID]graph.Ref{graph.ID(vmKind.Name, "vm-1"): {Kind: vmKind.Name, UUID: "vm-1"}}), "the vm the graph made of it")
	assert.Equal(t, "vm-1", blocks.ParentOf(choice, nil), "or the one it names")
	assert.Equal(t, "vm-1", blocks.ParentOf(choice, map[graph.NodeID]graph.Ref{graph.ID(vmKind.Name, "vm-1"): {Kind: containerKind.Name, UUID: "c-1"}}), "and what is not a vm is not its vm")
}
