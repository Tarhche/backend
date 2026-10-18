package reconcile_test

import (
	"context"
	"slices"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/khanzadimahdi/testproject/application/workload/controlplane/graph/reconcile"
	"github.com/khanzadimahdi/testproject/domain/workload/graph"
	"github.com/khanzadimahdi/testproject/domain/workload/graph/graphtest"
	containerKind "github.com/khanzadimahdi/testproject/domain/workload/kinds/container"
)

func TestReconciler_Enqueue(t *testing.T) {
	t.Parallel()

	api := graph.Ref{Kind: containerKind.Name, UUID: "api"}

	t.Run("a root is queued once however often it is queued, in the order roots were first queued", func(t *testing.T) {
		t.Parallel()

		f := newFixture(t, container())
		r := f.reconciler(t, "one", reconcile.DefaultConfig(), containerKind.Name)

		r.Enqueue(refOf(web))
		r.Enqueue(refOf(web))
		r.Enqueue(api)
		r.Enqueue(refOf(web))
		r.Enqueue(api)

		assert.Equal(t, []graph.Ref{refOf(web), api}, r.Queued())
	})

	t.Run("what is not of a kind of root here is not queued, nor what is named by no uuid", func(t *testing.T) {
		t.Parallel()

		f := newFixture(t, container())
		r := f.reconciler(t, "one", reconcile.DefaultConfig(), containerKind.Name)

		r.Enqueue(refOf(network))
		r.Enqueue(graph.Ref{Kind: "task", UUID: "t-uuid"})
		r.Enqueue(graph.Ref{Kind: containerKind.Name})

		assert.Empty(t, r.Queued())
	})

	t.Run("with no kinds of root, nothing is", func(t *testing.T) {
		t.Parallel()

		f := newFixture(t, container())
		r := f.reconciler(t, "one", reconcile.DefaultConfig())

		r.Enqueue(refOf(web))

		assert.Empty(t, r.Queued())
	})
}

func TestReconciler_Work(t *testing.T) {
	t.Parallel()

	t.Run("its workers reconcile what is queued, until they are stopped", func(t *testing.T) {
		t.Parallel()

		api := graph.ID(containerKind.Name, "api")

		kinds := container()
		kinds.Require(api, requirement(vm))
		kinds.Hold(vm, network, volume)

		f := newFixture(t, kinds, root(), in(refOf(api)))
		r := f.reconciler(t, "one", reconcile.DefaultConfig(), containerKind.Name)

		r.Enqueue(refOf(web))
		r.Enqueue(refOf(api))

		ctx, stop := context.WithCancel(context.Background())

		worked := make(chan struct{})
		go func() {
			defer close(worked)

			r.Work(ctx)
		}()

		require.Eventually(t, func() bool {
			return len(kinds.Asked(graphtest.Create)) == 2
		}, settle, tick, "both are made")

		assert.ElementsMatch(t, []graph.NodeID{web, api}, kinds.Asked(graphtest.Create))

		require.Eventually(t, func() bool {
			for _, root := range []string{"web", "api"} {
				if lease, kept := f.leases.Lease(root); !kept || len(lease.Holder) > 0 {
					return false
				}
			}

			return true
		}, settle, tick, "and each lease let go of")

		assert.Empty(t, r.Queued())

		stop()

		select {
		case <-worked:
		case <-time.After(settle):
			require.Fail(t, "the workers never stopped")
		}
	})

	t.Run("a root queued while it is reconciled is reconciled once more once it is done, and never by two workers at once", func(t *testing.T) {
		t.Parallel()

		gate := newGate()

		var (
			at   atomic.Int32
			most atomic.Int32
		)

		kinds := container()
		kinds.Hold(vm, network, volume, web)

		// how many look for it at once, the first held up until the gate
		// opens.
		kinds.On(graphtest.Exists, web, func(ctx context.Context) error {
			now := at.Add(1)
			defer at.Add(-1)

			for {
				seen := most.Load()
				if now <= seen || most.CompareAndSwap(seen, now) {
					break
				}
			}

			time.Sleep(time.Millisecond)

			return gate.wait(ctx)
		})

		f := newFixture(t, kinds, root())

		config := reconcile.DefaultConfig()
		config.Workers = 4

		r := f.reconciler(t, "one", config, containerKind.Name)

		ctx, stop := context.WithCancel(context.Background())
		defer stop()

		worked := make(chan struct{})
		go func() {
			defer close(worked)

			r.Work(ctx)
		}()

		r.Enqueue(refOf(web))

		require.Eventually(t, func() bool { return slices.Contains(kinds.Asked(graphtest.Exists), web) }, settle, tick, "a worker reconciles it")

		for range 3 {
			r.Enqueue(refOf(web))
		}

		assert.Empty(t, r.Queued(), "it waits for its reconcile to be done, and for no worker")

		gate.Open()

		require.Eventually(t, func() bool {
			return len(slices.DeleteFunc(kinds.Asked(graphtest.Exists), func(id graph.NodeID) bool { return id != web })) == 2
		}, settle, tick, "it is reconciled once more")

		// long enough for any more to show.
		time.Sleep(50 * time.Millisecond)

		looked := slices.DeleteFunc(kinds.Asked(graphtest.Exists), func(id graph.NodeID) bool { return id != web })
		assert.Len(t, looked, 2, "and only once")
		assert.Equal(t, int32(1), most.Load(), "by one worker at a time")

		stop()
		<-worked
	})

	t.Run("stopped, its workers take nothing more up, and what is queued waits", func(t *testing.T) {
		t.Parallel()

		f := newFixture(t, container(), root())
		r := f.reconciler(t, "one", reconcile.DefaultConfig(), containerKind.Name)

		r.Enqueue(refOf(web))

		ctx, stop := context.WithCancel(context.Background())
		stop()

		r.Work(ctx)

		assert.Empty(t, f.kinds.Calls())
		assert.Equal(t, []graph.Ref{refOf(web)}, r.Queued())
	})
}
