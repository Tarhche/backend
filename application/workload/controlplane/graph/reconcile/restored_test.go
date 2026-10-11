package reconcile_test

import (
	"context"
	"errors"
	"slices"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/khanzadimahdi/testproject/application/workload/controlplane/graph/reconcile"
	"github.com/khanzadimahdi/testproject/domain"
	"github.com/khanzadimahdi/testproject/domain/workload/graph"
	"github.com/khanzadimahdi/testproject/domain/workload/graph/graphtest"
	"github.com/khanzadimahdi/testproject/domain/workload/kind"
	containerKind "github.com/khanzadimahdi/testproject/domain/workload/kinds/container"
	networkKind "github.com/khanzadimahdi/testproject/domain/workload/kinds/network"
	vmKind "github.com/khanzadimahdi/testproject/domain/workload/kinds/vm"
	volumeKind "github.com/khanzadimahdi/testproject/domain/workload/kinds/volume"
	"github.com/khanzadimahdi/testproject/domain/workload/resource"
)

// reset is r reset by a restore of what it lives in.
func reset(r resource.Record) resource.Record {
	r.Reset = true

	return r
}

func TestReconciler_Reconcile_restored(t *testing.T) {
	t.Parallel()

	ctx := context.Background()

	t.Run("a root a restore of what it lives in reset waits, nothing of its plan carried out, while its kind does not know yet what the restored parent holds of it", func(t *testing.T) {
		t.Parallel()

		// its network is missing, which carrying its plan out would make.
		kinds := container()
		kinds.Hold(vm, volume, web)

		f := newFixture(t, kinds, reset(root()))
		k := f.resetting(graph.RestorationUnknown)

		r := f.reconciler(t, "one", reconcile.DefaultConfig(), containerKind.Name)

		require.NoError(t, r.Reconcile(ctx, refOf(web)))

		assert.Equal(t, 1, k.Asked())
		assert.Empty(t, kinds.Calls(), "nothing of its plan is planned, looked for or made")
		assert.Empty(t, f.kept(t, web))

		rec, err := f.records.GetOne(ctx, containerKind.Name, "web")
		require.NoError(t, err)
		assert.True(t, rec.Reset, "it is reset still")

		lease := f.lease(t, web)
		assert.Empty(t, lease.Holder)
		assert.Zero(t, lease.Failures, "waiting is no failure")
	})

	t.Run("one its kind finds the restored parent holds is no longer reset, and is carried out as before, making what of it the parent does not hold", func(t *testing.T) {
		t.Parallel()

		kinds := container()
		kinds.Hold(vm, volume, web)

		f := newFixture(t, kinds, reset(root()))
		k := f.resetting(graph.RestorationHeld)

		r := f.reconciler(t, "one", reconcile.DefaultConfig(), containerKind.Name)

		require.NoError(t, r.Reconcile(ctx, refOf(web)))

		rec, err := f.records.GetOne(ctx, containerKind.Name, "web")
		require.NoError(t, err)
		assert.False(t, rec.Reset)

		assert.Equal(t, []graph.NodeID{network}, kinds.Asked(graphtest.Create), "what of it the restored parent does not hold is made again")
		assert.Equal(t, map[graph.NodeID]graph.State{vm: graph.StateDone, network: graph.StateDone, volume: graph.StateDone, web: graph.StateDone}, states(f.kept(t, web)))

		require.NoError(t, r.Reconcile(ctx, refOf(web)))
		assert.Equal(t, 1, k.Asked(), "and its kind is asked no more, as it is not reset any more")
	})

	t.Run("one written meanwhile is read again, and what its kind finds is asked again", func(t *testing.T) {
		t.Parallel()

		kinds := container()
		kinds.Hold(vm, network, volume, web)

		f := newFixture(t, kinds, reset(root()))
		k := f.resetting(graph.RestorationHeld)

		// a person's action written as its kind was asked.
		var once sync.Once
		k.first = func(ctx context.Context) {
			once.Do(func() {
				rec, err := f.records.GetOne(ctx, containerKind.Name, "web")
				require.NoError(t, err)

				rec.Metadata.Name = "renamed"

				_, err = f.records.Update(ctx, rec)
				require.NoError(t, err)
			})
		}

		r := f.reconciler(t, "one", reconcile.DefaultConfig(), containerKind.Name)

		require.NoError(t, r.Reconcile(ctx, refOf(web)))

		assert.Equal(t, 2, k.Asked())

		rec, err := f.records.GetOne(ctx, containerKind.Name, "web")
		require.NoError(t, err)
		assert.False(t, rec.Reset)
		assert.Equal(t, "renamed", rec.Metadata.Name, "what was written meanwhile is kept")
	})

	t.Run("one its kind finds the restored parent does not hold is forgotten, with what it owns, and nothing is asked of a kind, nor of a node", func(t *testing.T) {
		t.Parallel()

		api := graph.ID(containerKind.Name, "api")
		shared := graph.ID(networkKind.Name, "s_shared")

		kinds := container()
		kinds.Hold(vm, network, volume, web, api, shared)

		f := newFixture(t, kinds,
			record(refOf(vm), nil, nil),
			in(refOf(network), refOf(web)),
			in(refOf(volume)),
			reset(root(func(m *kind.Metadata) { m.Refs = []graph.Ref{refOf(vm), refOf(volume)} })),
			record(refOf(api), []graph.Ref{refOf(vm)}, nil),
			in(refOf(shared), refOf(web), refOf(api)),
		)

		f.resetting(graph.RestorationLost)

		// how a run of it went before.
		require.NoError(t, f.executions.Save(ctx, graph.Execution{Root: refOf(web), Node: web, Kind: containerKind.Name, Action: graph.ActionCreate, State: graph.StateDone}))

		r := f.reconciler(t, "one", reconcile.DefaultConfig(), containerKind.Name)

		require.NoError(t, r.Reconcile(ctx, refOf(web)))

		for kindName, uuid := range map[string]string{containerKind.Name: "web", networkKind.Name: "s_default"} {
			_, err := f.records.GetOne(ctx, kindName, uuid)
			assert.ErrorIs(t, err, domain.ErrNotExists, "%s/%s is forgotten", kindName, uuid)
		}

		for kindName, uuid := range map[string]string{vmKind.Name: "v-uuid", volumeKind.Name: "s_data", networkKind.Name: "s_shared", containerKind.Name: "api"} {
			_, err := f.records.GetOne(ctx, kindName, uuid)
			assert.NoError(t, err, "%s/%s, which it only uses, or which what stays owns too, stays", kindName, uuid)
		}

		assert.Empty(t, kinds.Asked(graphtest.Delete), "nothing is asked of a kind, which would ask its node")
		assert.Empty(t, kinds.Asked(graphtest.Forget))

		writes := f.resources.Writes()
		require.NotEmpty(t, writes)
		assert.Equal(t, "delete container/web", writes[len(writes)-1], "its record goes last of all")

		_, kept := f.leases.Lease("web")
		assert.False(t, kept, "its lease is forgotten")
		assert.Empty(t, f.kept(t, web), "and so is how its plan went")
	})

	t.Run("a forget that finds it forgotten already, by another control plane, fails nothing", func(t *testing.T) {
		t.Parallel()

		kinds := container()
		kinds.Hold(vm, network, volume, web)

		f := newFixture(t, kinds, record(refOf(vm), nil, nil), in(refOf(network), refOf(web)), reset(root()))

		k := f.resetting(graph.RestorationLost)
		k.first = func(ctx context.Context) {
			require.NoError(t, f.records.Delete(ctx, networkKind.Name, "s_default"))
			require.NoError(t, f.records.Delete(ctx, containerKind.Name, "web"))
		}

		r := f.reconciler(t, "one", reconcile.DefaultConfig(), containerKind.Name)

		require.NoError(t, r.Reconcile(ctx, refOf(web)))

		_, kept := f.leases.Lease("web")
		assert.False(t, kept)
		assert.Empty(t, kinds.Asked(graphtest.Delete))
	})

	t.Run("one asked to go is removed as any root is, its kind not asked what the restored parent holds of it", func(t *testing.T) {
		t.Parallel()

		kinds := container()
		kinds.Hold(vm, network, volume, web)

		f := newFixture(t, kinds, reset(root(deleting)))
		k := f.resetting(graph.RestorationUnknown)

		kinds.On(graphtest.Forget, web, func(ctx context.Context) error {
			return f.records.Delete(ctx, containerKind.Name, "web")
		})

		r := f.reconciler(t, "one", reconcile.DefaultConfig(), containerKind.Name)

		require.NoError(t, r.Reconcile(ctx, refOf(web)))

		assert.Zero(t, k.Asked())
		assert.Contains(t, kinds.Asked(graphtest.Delete), web, "removed through its kind, as any root is")
	})

	t.Run("a run carrying one out when a restore resets it makes nothing more, saying why", func(t *testing.T) {
		t.Parallel()

		// its network is missing, which the run would make.
		kinds := container()
		kinds.Hold(vm, volume, web)

		f := newFixture(t, kinds, root())
		f.resetting(graph.RestorationUnknown)

		// its vm restored once the run looked its network for.
		kinds.On(graphtest.Exists, network, func(ctx context.Context) error {
			rec, err := f.records.GetOne(ctx, containerKind.Name, "web")
			if err != nil {
				return err
			}

			_, err = f.records.Update(ctx, reset(rec))

			return err
		})

		r := f.reconciler(t, "one", reconcile.DefaultConfig(), containerKind.Name)

		require.NoError(t, r.Reconcile(ctx, refOf(web)))

		assert.Empty(t, kinds.Asked(graphtest.Create), "nothing is made")

		waited := slices.IndexFunc(f.kept(t, web), func(e graph.Execution) bool { return e.Node == network })
		require.GreaterOrEqual(t, waited, 0)

		assert.Equal(t, graph.StateWaiting, f.kept(t, web)[waited].State)
		assert.Equal(t, "its root's vm was restored, and what the restored vm holds of its root is yet to be known", f.kept(t, web)[waited].Reason)
	})

	t.Run("what telling what the restored parent holds of it fails with is a failure, which backs it off and leaves it as it was", func(t *testing.T) {
		t.Parallel()

		kinds := container()
		kinds.Hold(vm, network, volume, web)

		f := newFixture(t, kinds, reset(root()))
		k := f.resetting(graph.RestorationHeld)
		k.fails = errors.New("the database is gone")

		r := f.reconciler(t, "one", reconcile.DefaultConfig(), containerKind.Name)

		require.ErrorIs(t, r.Reconcile(ctx, refOf(web)), k.fails)

		assert.Equal(t, 1, f.lease(t, web).Failures)

		rec, err := f.records.GetOne(ctx, containerKind.Name, "web")
		require.NoError(t, err)
		assert.True(t, rec.Reset)
		assert.Empty(t, kinds.Calls())
	})
}
