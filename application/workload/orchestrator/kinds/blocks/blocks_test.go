package blocks_test

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/khanzadimahdi/testproject/application/workload/orchestrator/kinds/blocks"
	"github.com/khanzadimahdi/testproject/application/workload/orchestrator/kinds/blocks/blockstest"
	"github.com/khanzadimahdi/testproject/domain/workload/docker"
	"github.com/khanzadimahdi/testproject/domain/workload/kind"
	"github.com/khanzadimahdi/testproject/domain/workload/vm"
)

func TestReader_Read(t *testing.T) {
	t.Parallel()

	t.Run("every running docker vm is read whole, once, all at once; the rest are not read at all", func(t *testing.T) {
		t.Parallel()

		node := blockstest.NewNode()
		first := node.DockerVM(t, "vm-1")
		second := node.DockerVM(t, "vm-2")
		node.DockerVM(t, "vm-stopped")
		node.Stop(t, "vm-stopped")
		node.Machine(t, "vm-machine")

		first.Hold(docker.Container{ID: "c1", Name: "web", State: "running"})

		read, err := blocks.NewReader(node.Engine, node).Read(t.Context())
		require.NoError(t, err)

		assert.Equal(t, []string{"vm-1", "vm-2"}, read.VMs())
		assert.Empty(t, read.Unseen)
		assert.Len(t, read.Inventories["vm-1"].Containers, 1)

		assert.Equal(t, 1, first.Calls("Inventory"), "one inventory of each vm")
		assert.Equal(t, 1, second.Calls("Inventory"))
	})

	t.Run("a vm whose dockerd does not answer is unseen, and the others are read", func(t *testing.T) {
		t.Parallel()

		node := blockstest.NewNode()
		node.DockerVM(t, "vm-1")
		node.DockerVM(t, "vm-2").Down = true

		read, err := blocks.NewReader(node.Engine, node).Read(t.Context())
		require.NoError(t, err)

		assert.Equal(t, []string{"vm-1"}, read.VMs())
		assert.Equal(t, []string{"vm-2"}, read.Unseen)
	})

	t.Run("and so is one that does not answer before the beat is over", func(t *testing.T) {
		t.Parallel()

		node := blockstest.NewNode()
		node.DockerVM(t, "vm-1")
		node.DockerVM(t, "vm-slow").Delay = time.Minute

		ctx, cancel := context.WithTimeout(t.Context(), 500*time.Millisecond)
		defer cancel()

		started := time.Now()

		read, err := blocks.NewReader(node.Engine, node).Read(ctx)
		require.NoError(t, err)

		assert.Less(t, time.Since(started), 500*time.Millisecond, "given up on a little before the beat is")
		assert.Equal(t, []string{"vm-1"}, read.VMs())
		assert.Equal(t, []string{"vm-slow"}, read.Unseen)
	})

	t.Run("the kinds asked in one beat share one read, and the next beat makes its own", func(t *testing.T) {
		t.Parallel()

		node := blockstest.NewNode()
		dockerd := node.DockerVM(t, "vm-1")
		dockerd.Delay = 50 * time.Millisecond

		reader := blocks.NewReader(node.Engine, node)
		beat := kind.WithBeat(t.Context(), time.Now())

		var wg sync.WaitGroup
		for range 4 {
			wg.Go(func() {
				read, err := reader.Read(beat)
				assert.NoError(t, err)
				assert.Equal(t, []string{"vm-1"}, read.VMs())
			})
		}

		wg.Wait()

		_, err := reader.Read(beat)
		require.NoError(t, err)

		assert.Equal(t, 1, dockerd.Calls("Inventory"), "four kinds, and one more asking in the same beat, and one read")

		_, err = reader.Read(kind.WithBeat(t.Context(), time.Now()))
		require.NoError(t, err)

		assert.Equal(t, 2, dockerd.Calls("Inventory"), "a beat after it says what is there since")
	})

	t.Run("a read begun before a beat is not that beat's, which says what is there since", func(t *testing.T) {
		t.Parallel()

		node := blockstest.NewNode()
		dockerd := node.DockerVM(t, "vm-1")
		dockerd.Hold(docker.Container{ID: "c1", Name: "later", State: "running"})

		reader := blocks.NewReader(node.Engine, node)

		before, err := reader.Read(kind.WithBeat(t.Context(), time.Now()))
		require.NoError(t, err)
		require.Len(t, before.Inventories["vm-1"].Containers, 1)

		// its disk restored from a snapshot that has none of it.
		dockerd.Forget("later")

		since, err := reader.Read(kind.WithBeat(t.Context(), time.Now()))
		require.NoError(t, err)
		assert.Empty(t, since.Inventories["vm-1"].Containers)
		assert.False(t, since.At.Before(before.At))
	})

	t.Run("and anything else asking is given a read begun since it asked", func(t *testing.T) {
		t.Parallel()

		node := blockstest.NewNode()
		dockerd := node.DockerVM(t, "vm-1")

		reader := blocks.NewReader(node.Engine, node)

		for range 2 {
			_, err := reader.Read(t.Context())
			require.NoError(t, err)
		}

		assert.Equal(t, 2, dockerd.Calls("Inventory"))
	})
}

func TestReport(t *testing.T) {
	t.Parallel()

	report := blocks.Report[kind.Status](blocks.Read{
		Inventories: map[string]docker.Inventory{"vm-2": {}, "vm-1": {}},
		Unseen:      []string{"vm-3"},
	})

	assert.Equal(t, []string{"vm-1", "vm-2"}, report.Read, "what it looked inside")
	assert.Equal(t, []string{"vm-3"}, report.Unseen)
	assert.NotNil(t, report.Instances, "and nothing yet, which is not nothing at all")
}

func TestReader_Reach(t *testing.T) {
	t.Parallel()

	t.Run("a running docker vm's dockerd, once it answers", func(t *testing.T) {
		t.Parallel()

		node := blockstest.NewNode()
		dockerd := node.DockerVM(t, "vm-1")

		daemon, err := blocks.NewReader(node.Engine, node).Reach(t.Context(), "vm-1")
		require.NoError(t, err)

		assert.Same(t, dockerd, daemon)
		assert.Equal(t, 1, dockerd.Calls("Ping"), "it was waited for")
	})

	t.Run("one that is not here is gone, and not running", func(t *testing.T) {
		t.Parallel()

		_, err := blocks.NewReader(blockstest.NewNode().Engine, blockstest.NewNode()).Reach(t.Context(), "vm-elsewhere")

		assert.ErrorIs(t, err, blocks.ErrGone)
		assert.ErrorIs(t, err, vm.ErrNotRunning)
	})

	t.Run("one that is stopped is not running", func(t *testing.T) {
		t.Parallel()

		node := blockstest.NewNode()
		node.DockerVM(t, "vm-1")
		node.Stop(t, "vm-1")

		_, err := blocks.NewReader(node.Engine, node).Reach(t.Context(), "vm-1")

		assert.ErrorIs(t, err, vm.ErrNotRunning)
		assert.NotErrorIs(t, err, blocks.ErrGone)
	})

	t.Run("one whose dockerd does not come up is unavailable", func(t *testing.T) {
		t.Parallel()

		node := blockstest.NewNode()
		node.DockerVM(t, "vm-1").Down = true

		_, err := blocks.NewReader(node.Engine, node).Reach(t.Context(), "vm-1")

		assert.ErrorIs(t, err, docker.ErrUnavailable)
	})

	t.Run("a vm named by a command is read as a docker vm, whatever its labels say", func(t *testing.T) {
		t.Parallel()

		node := blockstest.NewNode()
		node.Machine(t, "vm-old")

		reader := blocks.NewReader(node.Engine, node)

		read, err := reader.Read(t.Context())
		require.NoError(t, err)
		assert.Empty(t, read.VMs(), "a vm not labelled as a docker vm is not read")

		_, err = reader.Reach(t.Context(), "vm-old")
		assert.ErrorIs(t, err, docker.ErrUnavailable, "it has no dockerd here")

		read, err = reader.Read(t.Context())
		require.NoError(t, err)
		assert.Equal(t, []string{"vm-old"}, read.Unseen, "once a command named it, it is looked into")
	})
}

func TestOwners(t *testing.T) {
	t.Parallel()

	stacks := blocks.Stacks(docker.Inventory{Containers: []docker.Container{
		{Labels: map[string]string{docker.LabelComposeProject: "shop-abcde", "workload.stack": "stack-uuid"}},
		{Labels: map[string]string{docker.LabelComposeProject: "elsewhere"}},
	}})

	assert.Equal(t, map[string]string{"shop-abcde": "stack-uuid"}, stacks)

	assert.Equal(t, []kind.Reference{{Kind: "vm", UUID: "vm-1"}, {Kind: "stack", UUID: "stack-uuid"}},
		blocks.Owners("vm-1", map[string]string{docker.LabelComposeProject: "shop-abcde"}, stacks),
		"a network or a volume of a stack's project is the stack's")
	assert.Equal(t, []kind.Reference{{Kind: "vm", UUID: "vm-1"}},
		blocks.Owners("vm-1", map[string]string{docker.LabelComposeProject: "elsewhere"}, stacks),
		"and one of a project no stack is known by is the vm's alone")
	assert.Equal(t, []kind.Reference{{Kind: "vm", UUID: "vm-1"}, {Kind: "stack", UUID: "labelled"}},
		blocks.Owners("vm-1", map[string]string{"workload.stack": "labelled"}, stacks))
}
