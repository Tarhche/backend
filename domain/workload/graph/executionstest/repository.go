// Package executionstest holds a graph.Executions to what every one of them
// promises, so that the one kept in memory, which the executor's tests run
// on, and the one kept in MongoDB, which the control plane serves on, cannot
// drift apart.
package executionstest

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/khanzadimahdi/testproject/domain/workload/graph"
)

// Maker is a new repository of executions, with nothing kept in it yet.
type Maker func(t *testing.T) graph.Executions

// The roots the suite keeps the executions of: two, to hold them apart.
var (
	stack     = graph.Ref{Kind: "stack", UUID: "0193b5c2-7b1e-7000-8000-000000000001"}
	container = graph.Ref{Kind: "container", UUID: "0193b5c2-7b1e-7000-8000-000000000002"}
)

// The nodes of their plans: a stack's VM, network, volume and service, and
// the stack itself; a container's VM, which is the stack's too, and the
// container itself.
var (
	vm      = graph.ID("vm", "0193b5c2-7b1e-7000-8000-00000000000a")
	network = graph.ID("network", "shop_default")
	volume  = graph.ID("volume", "shop_data")
	web     = graph.ID("container", "shop_web")
	shop    = graph.ID("stack", stack.UUID)
	cache   = graph.ID("container", container.UUID)
)

// Repository fails t for everything the repository make makes does otherwise
// than every repository of executions has to.
func Repository(t *testing.T, make Maker) {
	t.Helper()

	t.Run("an execution is kept as it was given, its times to the millisecond and in UTC", func(t *testing.T) {
		t.Parallel()

		ctx := context.Background()
		repository := make(t)

		given := anExecution(stack, web, 2)
		require.NoError(t, repository.Save(ctx, given))

		kept, err := repository.GetByRoot(ctx, stack.UUID)
		require.NoError(t, err)

		assert.Equal(t, []graph.Execution{asKept(given)}, kept)
	})

	t.Run("saved again, it is written over, by its root's uuid and its node, every field of it, those left empty too", func(t *testing.T) {
		t.Parallel()

		ctx := context.Background()
		repository := make(t)

		require.NoError(t, repository.Save(ctx, anExecution(stack, web, 2)))

		// started again, by a plan of the root's next generation, in which
		// the node is a level lower.
		again := graph.Execution{
			Root:       stack,
			Generation: 4,
			Node:       web,
			Kind:       "container",
			Action:     graph.ActionCreate,
			Level:      1,
			State:      graph.StatePending,
			UpdatedAt:  moment(40),
		}
		require.NoError(t, repository.Save(ctx, again))

		kept, err := repository.GetByRoot(ctx, stack.UUID)
		require.NoError(t, err)

		assert.Equal(t, []graph.Execution{asKept(again)}, kept, "no reason, record or times are kept of what it was")
	})

	t.Run("a root's are read in the order of their levels, and then of their nodes, and nobody else's are", func(t *testing.T) {
		t.Parallel()

		ctx := context.Background()
		repository := make(t)

		// one VM is both roots', and a node of each of their plans.
		theirs := anExecution(container, vm, 0)
		theirs.State = graph.StateWaiting
		theirs.Reason = "it is being deleted"

		for _, e := range []graph.Execution{
			anExecution(stack, web, 2),
			anExecution(container, cache, 1),
			anExecution(stack, volume, 1),
			anExecution(stack, shop, 3),
			theirs,
			anExecution(stack, vm, 0),
			anExecution(stack, network, 1),
		} {
			require.NoError(t, repository.Save(ctx, e))
		}

		kept, err := repository.GetByRoot(ctx, stack.UUID)
		require.NoError(t, err)

		assert.Equal(t, []graph.Execution{
			asKept(anExecution(stack, vm, 0)),
			asKept(anExecution(stack, network, 1)),
			asKept(anExecution(stack, volume, 1)),
			asKept(anExecution(stack, web, 2)),
			asKept(anExecution(stack, shop, 3)),
		}, kept)

		kept, err = repository.GetByRoot(ctx, container.UUID)
		require.NoError(t, err)

		assert.Equal(t, []graph.Execution{asKept(theirs), asKept(anExecution(container, cache, 1))}, kept, "the VM as the container's plan went")
	})

	t.Run("a root's are forgotten together, and nobody else's are", func(t *testing.T) {
		t.Parallel()

		ctx := context.Background()
		repository := make(t)

		for _, e := range []graph.Execution{
			anExecution(stack, vm, 0),
			anExecution(stack, web, 1),
			anExecution(container, vm, 0),
			anExecution(container, cache, 1),
		} {
			require.NoError(t, repository.Save(ctx, e))
		}

		require.NoError(t, repository.DeleteByRoot(ctx, stack.UUID))

		kept, err := repository.GetByRoot(ctx, stack.UUID)
		require.NoError(t, err)
		assert.Empty(t, kept)

		kept, err = repository.GetByRoot(ctx, container.UUID)
		require.NoError(t, err)
		assert.Equal(t, []graph.Execution{asKept(anExecution(container, vm, 0)), asKept(anExecution(container, cache, 1))}, kept)

		assert.NoError(t, repository.DeleteByRoot(ctx, stack.UUID), "what is forgotten is forgotten already")
	})

	t.Run("a root of which nothing is kept has nothing to read", func(t *testing.T) {
		t.Parallel()

		ctx := context.Background()
		repository := make(t)

		require.NoError(t, repository.Save(ctx, anExecution(stack, vm, 0)))

		kept, err := repository.GetByRoot(ctx, container.UUID)
		require.NoError(t, err)
		assert.Empty(t, kept)

		assert.NoError(t, repository.DeleteByRoot(ctx, container.UUID))
	})

	t.Run("an execution that names no root or no node is not kept", func(t *testing.T) {
		t.Parallel()

		ctx := context.Background()
		repository := make(t)

		rootless := anExecution(graph.Ref{Kind: "stack"}, web, 2)
		assert.Error(t, repository.Save(ctx, rootless))

		nodeless := anExecution(stack, "", 2)
		assert.Error(t, repository.Save(ctx, nodeless))

		kept, err := repository.GetByRoot(ctx, "")
		require.NoError(t, err)
		assert.Empty(t, kept)

		kept, err = repository.GetByRoot(ctx, stack.UUID)
		require.NoError(t, err)
		assert.Empty(t, kept)
	})
}

// anExecution is the node of the root's plan at the level, with everything an
// execution can have: done, with its record, a reason, and every time, each
// to the nanosecond, and in Tehran.
func anExecution(root graph.Ref, node graph.NodeID, level int) graph.Execution {
	return graph.Execution{
		Root:       root,
		Generation: 3,
		Node:       node,
		Kind:       node.Kind(),
		Action:     graph.ActionCreate,
		Level:      level,
		State:      graph.StateDone,
		Reason:     "it was there already",
		Ref:        graph.Ref{Kind: node.Kind(), UUID: node.Name() + "-uuid"},
		StartedAt:  moment(10),
		FinishedAt: moment(20),
		UpdatedAt:  moment(20),
	}
}

// moment is a time minutes past noon, to the nanosecond, and not in UTC.
func moment(minutes int) time.Time {
	return time.Date(2026, 10, 10, 12, minutes, 15, 123_456_789, time.FixedZone("Tehran", 3*60*60+30*60))
}

// asKept is an execution as a repository keeps it: its times to the
// millisecond, and in UTC.
func asKept(e graph.Execution) graph.Execution {
	e.StartedAt = millisecond(e.StartedAt)
	e.FinishedAt = millisecond(e.FinishedAt)
	e.UpdatedAt = millisecond(e.UpdatedAt)

	return e
}

func millisecond(t time.Time) time.Time {
	if t.IsZero() {
		return time.Time{}
	}

	return t.UTC().Truncate(time.Millisecond)
}
