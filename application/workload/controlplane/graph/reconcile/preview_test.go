package reconcile_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/khanzadimahdi/testproject/application/workload/controlplane/graph/execute"
	"github.com/khanzadimahdi/testproject/application/workload/controlplane/graph/reconcile"
	"github.com/khanzadimahdi/testproject/domain"
	"github.com/khanzadimahdi/testproject/domain/workload/graph"
	"github.com/khanzadimahdi/testproject/domain/workload/graph/graphtest"
	"github.com/khanzadimahdi/testproject/domain/workload/kind"
	containerKind "github.com/khanzadimahdi/testproject/domain/workload/kinds/container"
	networkKind "github.com/khanzadimahdi/testproject/domain/workload/kinds/network"
)

func TestReconciler_PreviewCreate(t *testing.T) {
	t.Parallel()

	ctx := context.Background()

	t.Run("what carrying a root out would do, of a record kept nowhere, is read and not done", func(t *testing.T) {
		t.Parallel()

		kinds := container()
		kinds.Hold(vm)

		f := newFixture(t, kinds)
		r := f.reconciler(t, "one", reconcile.DefaultConfig(), containerKind.Name)

		preview, err := r.PreviewCreate(ctx, root().Raw)
		require.NoError(t, err)

		verdicts := make(map[graph.NodeID]execute.Verdict)
		levels := make(map[graph.NodeID]int)

		for level, nodes := range preview.Levels {
			for _, previewed := range nodes {
				verdicts[previewed.Node] = previewed.Verdict
				levels[previewed.Node] = level
			}
		}

		assert.Equal(t, map[graph.NodeID]execute.Verdict{vm: execute.Exists, network: execute.Create, volume: execute.Create, web: execute.Create}, verdicts)
		assert.Equal(t, map[graph.NodeID]int{vm: 0, network: 1, volume: 1, web: 2}, levels)

		assert.Empty(t, kinds.Asked(graphtest.Create), "nothing is made")
		assert.Empty(t, f.executions.Saved(), "nor saved")
		assert.Empty(t, f.resources.Writes(), "nor written")
		assert.Empty(t, r.Queued(), "nor queued")

		_, leased := f.leases.Lease(web.Name())
		assert.False(t, leased, "and no lease is taken")
	})

	t.Run("a record of a kind that is no root here is not previewed", func(t *testing.T) {
		t.Parallel()

		f := newFixture(t, container())
		r := f.reconciler(t, "one", reconcile.DefaultConfig(), containerKind.Name)

		_, err := r.PreviewCreate(ctx, kind.Raw{Kind: networkKind.Name, Metadata: kind.Metadata{UUID: "n-uuid"}})
		assert.ErrorIs(t, err, kind.ErrUnknownKind)
	})
}

func TestReconciler_PreviewRemove(t *testing.T) {
	t.Parallel()

	ctx := context.Background()

	t.Run("what removing a root would do is read and not done", func(t *testing.T) {
		t.Parallel()

		kinds := container()
		kinds.Hold(vm, network, volume, web)
		kinds.Ownership().Own(network, web)

		f := newFixture(t, kinds, root())
		r := f.reconciler(t, "one", reconcile.DefaultConfig(), containerKind.Name)

		preview, err := r.PreviewRemove(ctx, refOf(web))
		require.NoError(t, err)

		verdicts := make(map[graph.NodeID]execute.Verdict)
		for _, nodes := range preview.Levels {
			for _, previewed := range nodes {
				verdicts[previewed.Node] = previewed.Verdict
			}
		}

		assert.Equal(t, map[graph.NodeID]execute.Verdict{web: execute.Delete, network: execute.Delete, vm: execute.Keep, volume: execute.Keep}, verdicts)

		assert.Empty(t, kinds.Asked(graphtest.Delete), "nothing is removed")
		assert.Empty(t, kinds.Asked(graphtest.Disown), "nor unlinked")
		assert.Empty(t, f.executions.Saved(), "nor saved")
		assert.Empty(t, f.resources.Writes(), "nor written")
		assert.ElementsMatch(t, []graph.NodeID{vm, network, volume, web}, kinds.Held(), "and all is held as it was")
	})

	t.Run("a root that is gone is not there", func(t *testing.T) {
		t.Parallel()

		f := newFixture(t, container())
		r := f.reconciler(t, "one", reconcile.DefaultConfig(), containerKind.Name)

		_, err := r.PreviewRemove(ctx, refOf(web))
		assert.ErrorIs(t, err, domain.ErrNotExists)
	})

	t.Run("nor is one of a kind that is no root here", func(t *testing.T) {
		t.Parallel()

		f := newFixture(t, container(), root())
		r := f.reconciler(t, "one", reconcile.DefaultConfig(), networkKind.Name)

		_, err := r.PreviewRemove(ctx, refOf(web))
		assert.ErrorIs(t, err, kind.ErrUnknownKind)
	})
}
