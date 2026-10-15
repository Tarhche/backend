package reconcile_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/khanzadimahdi/testproject/application/workload/controlplane/graph/reconcile"
	"github.com/khanzadimahdi/testproject/domain/workload/graph"
	"github.com/khanzadimahdi/testproject/domain/workload/graph/graphtest"
	containerKind "github.com/khanzadimahdi/testproject/domain/workload/kinds/container"
	networkKind "github.com/khanzadimahdi/testproject/domain/workload/kinds/network"
	stackKind "github.com/khanzadimahdi/testproject/domain/workload/kinds/stack"
	volumeKind "github.com/khanzadimahdi/testproject/domain/workload/kinds/volume"
)

func TestReconciler_Resync(t *testing.T) {
	t.Parallel()

	ctx := context.Background()

	// read two at a time, so that every root is paged through.
	config := reconcile.DefaultConfig()
	config.Batch = 2

	for name, tt := range map[string]struct {
		roots []string
		want  []graph.Ref
	}{
		"every root of the kinds of root, and nothing any root owns": {
			roots: []string{containerKind.Name},
			want:  []graph.Ref{c1, c2, stranded, c3},
		},
		"and the stacks too, when they are roots": {
			roots: []string{containerKind.Name, stackKind.Name},
			want:  []graph.Ref{c1, c2, stranded, c3, shop},
		},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			f := newFixture(t, container(), workload()...)
			r := f.reconciler(t, "one", config, tt.roots...)

			require.NoError(t, r.Resync(ctx))

			assert.ElementsMatch(t, tt.want, r.Queued())
			assert.Empty(t, f.kinds.Calls(), "nothing is reconciled yet, and nothing every owner of which is there is removed")
			assert.Empty(t, f.resources.Writes())
		})
	}

	t.Run("with no kinds of root, nothing is read, queued or removed", func(t *testing.T) {
		t.Parallel()

		gone := graph.Ref{Kind: stackKind.Name, UUID: "gone"}

		f := newFixture(t, container(), append(workload(), record(graph.Ref{Kind: containerKind.Name, UUID: "orphan"}, []graph.Ref{theVM}, []graph.Ref{gone}))...)
		r := f.reconciler(t, "one", config)

		require.NoError(t, r.Resync(ctx))

		assert.Empty(t, r.Queued())
		assert.Empty(t, f.kinds.Calls())
		assert.Zero(t, f.resources.Reads())
	})
}

func TestReconciler_Resync_collect(t *testing.T) {
	t.Parallel()

	ctx := context.Background()

	t.Run("what something owned, every owner of which is gone, is removed through its kind, and nothing else is", func(t *testing.T) {
		t.Parallel()

		gone := func(kindName string, uuid string) graph.Ref { return graph.Ref{Kind: kindName, UUID: uuid} }

		f := newFixture(t, container(),
			record(theVM, nil, nil),
			record(c1, []graph.Ref{theVM}, nil),

			// what nothing that owned it is left of.
			record(graph.Ref{Kind: networkKind.Name, UUID: "orphaned-network"}, []graph.Ref{theVM}, []graph.Ref{gone(containerKind.Name, "gone-1")}),
			record(graph.Ref{Kind: containerKind.Name, UUID: "orphaned-service"}, []graph.Ref{theVM}, []graph.Ref{gone(stackKind.Name, "gone-stack")}),
			record(graph.Ref{Kind: volumeKind.Name, UUID: "orphaned-volume"}, []graph.Ref{theVM}, []graph.Ref{gone(containerKind.Name, "gone-1"), gone(containerKind.Name, "gone-2")}),

			// what something there still owns.
			record(graph.Ref{Kind: networkKind.Name, UUID: "owned"}, []graph.Ref{theVM}, []graph.Ref{c1}),
			record(graph.Ref{Kind: networkKind.Name, UUID: "half-owned"}, []graph.Ref{theVM}, []graph.Ref{gone(containerKind.Name, "gone-1"), c1}),

			// what nothing ever owned.
			record(graph.Ref{Kind: volumeKind.Name, UUID: "free"}, []graph.Ref{theVM}, nil),
		)

		config := reconcile.DefaultConfig()
		config.Batch = 2

		r := f.reconciler(t, "one", config, containerKind.Name)

		require.NoError(t, r.Resync(ctx))

		assert.ElementsMatch(t, []graph.NodeID{
			graph.ID(containerKind.Name, "orphaned-service"),
			graph.ID(networkKind.Name, "orphaned-network"),
			graph.ID(volumeKind.Name, "orphaned-volume"),
		}, f.kinds.Asked(graphtest.Delete))
	})

	t.Run("what is owned by what is going waits for a later resync, once what owns it is gone", func(t *testing.T) {
		t.Parallel()

		gone := graph.Ref{Kind: stackKind.Name, UUID: "gone-stack"}
		serviceID := graph.ID(containerKind.Name, service.UUID)
		networkID := graph.ID(networkKind.Name, shopNet.UUID)

		kinds := container()

		f := newFixture(t, kinds,
			record(theVM, nil, nil),
			record(service, []graph.Ref{theVM}, []graph.Ref{gone}),
			record(shopNet, []graph.Ref{theVM}, []graph.Ref{gone, service}),
		)

		// the service cannot be removed at first; once it can, it goes, and
		// so does the network, as their kinds remove them.
		removable := false

		kinds.On(graphtest.Delete, serviceID, func(ctx context.Context) error {
			if !removable {
				return &graph.Waiting{Reason: "its vm is stopped"}
			}

			return f.records.Delete(ctx, containerKind.Name, service.UUID)
		})

		kinds.On(graphtest.Delete, networkID, func(ctx context.Context) error {
			return f.records.Delete(ctx, networkKind.Name, shopNet.UUID)
		})

		r := f.reconciler(t, "one", reconcile.DefaultConfig(), containerKind.Name)

		require.NoError(t, r.Resync(ctx), "what waits is no failure")
		assert.Equal(t, []graph.NodeID{serviceID}, kinds.Asked(graphtest.Delete), "the network waits, as the service that owns it is there")

		removable = true

		require.NoError(t, r.Resync(ctx))
		assert.Equal(t, []graph.NodeID{serviceID, serviceID, networkID}, kinds.Asked(graphtest.Delete), "and goes once the service is gone")

		require.NoError(t, r.Resync(ctx))
		assert.Len(t, kinds.Asked(graphtest.Delete), 3, "and nothing is left to remove")
	})
}
