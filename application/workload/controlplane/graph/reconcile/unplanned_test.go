package reconcile_test

import (
	"context"
	"errors"
	"slices"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/khanzadimahdi/testproject/application/workload/controlplane/graph/reconcile"
	"github.com/khanzadimahdi/testproject/application/workload/controlplane/kinds/kindstest"
	"github.com/khanzadimahdi/testproject/domain"
	"github.com/khanzadimahdi/testproject/domain/workload/graph"
	"github.com/khanzadimahdi/testproject/domain/workload/graph/graphtest"
	containerKind "github.com/khanzadimahdi/testproject/domain/workload/kinds/container"
	stackKind "github.com/khanzadimahdi/testproject/domain/workload/kinds/stack"
	"github.com/khanzadimahdi/testproject/domain/workload/resource"
)

func TestReconciler_Reconcile_unplanned(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	unreadable := errors.New("its file says build: .")

	// db is the stack store's other container, beside web, and other a root
	// of its own.
	db := graph.ID(containerKind.Name, "db")
	other := graph.ID(containerKind.Name, "other")

	// records are those of the stack store, asked to go, whose file cannot
	// be read: its containers web and db, owned by it, web mounting its
	// volume, which nothing owns; and its network, owned by it and by both,
	// and by the networks' owners, when there are more; all in the vm.
	records := func(networkOwners ...graph.Ref) []resource.Record {
		s := aStack(stackKind.Removing)
		deleting(&s.Metadata)

		return []resource.Record{
			s,
			record(refOf(web), []graph.Ref{refOf(vm), refOf(volume)}, []graph.Ref{refOf(store)}),
			record(refOf(db), []graph.Ref{refOf(vm)}, []graph.Ref{refOf(store)}),
			in(refOf(network), append([]graph.Ref{refOf(store), refOf(web), refOf(db)}, networkOwners...)...),
			in(refOf(volume)),
			record(refOf(vm), nil, nil),
		}
	}

	// unplanned are the kinds of what the records are, store's record saying
	// nothing its plan can be made of.
	unplanned := func() *graphtest.Kind {
		kinds := graphtest.NewKind()
		kinds.Hold(vm, network, volume, web, db, store, other)
		kinds.On(graphtest.Root, store, func(context.Context) error { return unreadable })

		return kinds
	}

	t.Run("a root asked to go whose plan cannot be made is removed through what its records say it owns: it first, then what it owns, each after what owns it or uses it, and then its record", func(t *testing.T) {
		t.Parallel()

		kinds := unplanned()

		f := newFixture(t, kinds, records()...)
		r := f.reconciler(t, "one", reconcile.DefaultConfig(), stackKind.Name)

		require.NoError(t, r.Reconcile(ctx, refOf(store)))

		assert.Equal(t, []graph.NodeID{store, db, web, network}, kinds.Asked(graphtest.Delete), "its containers before the network they join")
		assert.Equal(t, []graph.NodeID{other, vm, volume}, kinds.Held(), "and its volume and its vm, which it only uses, stay")

		_, err := f.records.GetOne(ctx, stackKind.Name, store.Name())
		assert.ErrorIs(t, err, domain.ErrNotExists, "its record goes last")

		_, kept := f.leases.Lease(store.Name())
		assert.False(t, kept, "and its lease with it")
		assert.Empty(t, f.kept(t, store))
	})

	t.Run("and a volume it owns, as a policy may have it, after what mounts it", func(t *testing.T) {
		t.Parallel()

		kinds := unplanned()

		owning := records()
		for i, r := range owning {
			if r.Kind == volume.Kind() {
				owning[i] = in(refOf(volume), refOf(store))
			}
		}

		f := newFixture(t, kinds, owning...)
		r := f.reconciler(t, "one", reconcile.DefaultConfig(), stackKind.Name)

		require.NoError(t, r.Reconcile(ctx, refOf(store)))

		deleted := kinds.Asked(graphtest.Delete)
		require.Len(t, deleted, 5)
		assert.Equal(t, []graph.NodeID{store, db, web}, deleted[:3])
		assert.ElementsMatch(t, []graph.NodeID{network, volume}, deleted[3:], "once web, which mounts it, is gone")
		assert.Equal(t, []graph.NodeID{other, vm}, kinds.Held())
	})

	t.Run("what something that stays owns too stays, and so does what it owns", func(t *testing.T) {
		t.Parallel()

		kinds := unplanned()

		f := newFixture(t, kinds, append(records(refOf(other)), record(refOf(other), []graph.Ref{refOf(vm)}, nil))...)
		r := f.reconciler(t, "one", reconcile.DefaultConfig(), stackKind.Name)

		require.NoError(t, r.Reconcile(ctx, refOf(store)))

		assert.Equal(t, []graph.NodeID{store, db, web}, kinds.Asked(graphtest.Delete))
		assert.Contains(t, kinds.Held(), network, "the network another root owns too stays")
	})

	t.Run("one whose removal waits waits, and is taken up where it stopped once it can be", func(t *testing.T) {
		t.Parallel()

		kinds := unplanned()
		kinds.On(graphtest.Delete, web, func(context.Context) error { return &graph.Waiting{Reason: "its vm is stopped"} })

		f := newFixture(t, kinds, records()...)
		r := f.reconciler(t, "one", reconcile.DefaultConfig(), stackKind.Name)

		require.NoError(t, r.Reconcile(ctx, refOf(store)), "it waits: it is no failure")

		assert.NotContains(t, kinds.Asked(graphtest.Delete), network, "nothing after it is removed")
		assert.Zero(t, f.lease(t, store).Failures)

		_, err := f.records.GetOne(ctx, stackKind.Name, store.Name())
		require.NoError(t, err, "its record is kept, for the next run")

		kinds.On(graphtest.Delete, web, nil)

		require.NoError(t, r.Reconcile(ctx, refOf(store)))

		assert.Contains(t, kinds.Asked(graphtest.Delete), network)

		_, err = f.records.GetOne(ctx, stackKind.Name, store.Name())
		assert.ErrorIs(t, err, domain.ErrNotExists)
	})

	t.Run("its removal holds its lease, renewed every third of its time, so that another control plane does not remove it at once, however long a delete of it takes", func(t *testing.T) {
		t.Parallel()

		config := reconcile.DefaultConfig()
		config.Lease = 30 * time.Millisecond

		gate := newGate()

		kinds := unplanned()
		kinds.On(graphtest.Delete, web, gate.wait)

		f := newFixture(t, kinds, records()...)

		one := f.reconciler(t, "one", config, stackKind.Name)
		another := f.reconciler(t, "another", config, stackKind.Name)

		done := make(chan error, 1)
		go func() { done <- one.Reconcile(ctx, refOf(store)) }()

		require.Eventually(t, func() bool { return slices.Contains(kinds.Asked(graphtest.Delete), web) }, settle, tick, "one is removing web")

		expires := func(at time.Time) func() bool {
			return func() bool {
				lease, _ := f.leases.Lease(store.Name())

				return lease.ExpiresAt.Equal(at)
			}
		}

		require.Eventually(t, expires(kindstest.Moment.Add(30*time.Millisecond)), settle, tick, "taken until its time from then")

		f.clock.Advance(20 * time.Millisecond)
		require.Eventually(t, expires(kindstest.Moment.Add(50*time.Millisecond)), settle, tick, "renewed until its time from now")

		// past what it was taken until.
		f.clock.Advance(20 * time.Millisecond)

		assert.ErrorIs(t, another.Reconcile(ctx, refOf(store)), graph.ErrHeld, "another skips it")
		assert.Equal(t, []graph.NodeID{store, db, web}, kinds.Asked(graphtest.Delete), "and removes nothing of it meanwhile")

		gate.Open()
		require.NoError(t, <-done)

		assert.Equal(t, []graph.NodeID{store, db, web, network}, kinds.Asked(graphtest.Delete), "each removed once")

		_, err := f.records.GetOne(ctx, stackKind.Name, store.Name())
		assert.ErrorIs(t, err, domain.ErrNotExists)
	})

	t.Run("and a lease lost while it is removed, which another control plane took once it lapsed, cuts it short", func(t *testing.T) {
		t.Parallel()

		config := reconcile.DefaultConfig()
		config.Lease = 30 * time.Millisecond

		kinds := unplanned()
		kinds.On(graphtest.Delete, web, func(ctx context.Context) error {
			<-ctx.Done()

			return ctx.Err()
		})

		f := newFixture(t, kinds, records()...)
		one := f.reconciler(t, "one", config, stackKind.Name)

		done := make(chan error, 1)
		go func() { done <- one.Reconcile(ctx, refOf(store)) }()

		require.Eventually(t, func() bool { return slices.Contains(kinds.Asked(graphtest.Delete), web) }, settle, tick)

		f.lapse(t, store, "another")

		var err error

		select {
		case err = <-done:
		case <-time.After(settle):
			require.Fail(t, "the removal never stopped")
		}

		require.ErrorIs(t, err, graph.ErrLost, "how it ended is not kept on what it does not hold")
		assert.ErrorContains(t, err, "cut short")
		assert.NotContains(t, kinds.Asked(graphtest.Delete), network, "and nothing after it is removed")

		assert.Equal(t, "another", f.lease(t, store).Holder)

		_, err = f.records.GetOne(ctx, stackKind.Name, store.Name())
		require.NoError(t, err, "its record is kept, for the run that holds its lease")
	})

	t.Run("but one not asked to go whose plan cannot be made fails, and nothing of it is removed", func(t *testing.T) {
		t.Parallel()

		kinds := unplanned()

		f := newFixture(t, kinds, aStack(stackKind.Running))
		r := f.reconciler(t, "one", reconcile.DefaultConfig(), stackKind.Name)

		require.ErrorIs(t, r.Reconcile(ctx, refOf(store)), unreadable)

		assert.Empty(t, kinds.Asked(graphtest.Delete))
		assert.Equal(t, 1, f.lease(t, store).Failures)
	})
}
