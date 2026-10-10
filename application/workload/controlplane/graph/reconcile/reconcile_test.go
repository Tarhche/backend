package reconcile_test

import (
	"context"
	"errors"
	"log/slog"
	"slices"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/khanzadimahdi/testproject/application/workload/controlplane/graph/execute"
	"github.com/khanzadimahdi/testproject/application/workload/controlplane/graph/ownership"
	"github.com/khanzadimahdi/testproject/application/workload/controlplane/graph/reconcile"
	"github.com/khanzadimahdi/testproject/application/workload/controlplane/kinds/kindstest"
	"github.com/khanzadimahdi/testproject/domain/workload/graph"
	"github.com/khanzadimahdi/testproject/domain/workload/graph/graphtest"
	"github.com/khanzadimahdi/testproject/domain/workload/kind"
	containerKind "github.com/khanzadimahdi/testproject/domain/workload/kinds/container"
	stackKind "github.com/khanzadimahdi/testproject/domain/workload/kinds/stack"
	taskKind "github.com/khanzadimahdi/testproject/domain/workload/kinds/task"
	vmKind "github.com/khanzadimahdi/testproject/domain/workload/kinds/vm"
)

// plain is a kind of a graph whose resources are never roots: it says
// nothing of what a root of it needs.
type plain struct {
	graph.Kind
}

func TestNew(t *testing.T) {
	t.Parallel()

	// dependencies are what a reconciler is given, as change says otherwise.
	dependencies := func(t *testing.T, change func(d *reconcile.Dependencies)) reconcile.Dependencies {
		kinds := graphtest.NewKind()
		f := newFixture(t, kinds)

		executor, err := execute.New(kinds.Kinds(every...), f.ownership, f.executions, nil)
		require.NoError(t, err)

		d := reconcile.Dependencies{
			Registry:   f.registry,
			Resources:  f.resources,
			Kinds:      kinds.Kinds(every...),
			Roots:      []string{containerKind.Name, stackKind.Name},
			Planner:    graph.NewPlanner(kinds.Kinds(every...), graph.DefaultPolicy()),
			Executor:   executor,
			Executions: f.executions,
			Leases:     f.leases,
			Holder:     "one",
			Logger:     slog.New(slog.DiscardHandler),
		}

		if change != nil {
			change(&d)
		}

		return d
	}

	for name, tt := range map[string]struct {
		dependencies func(d *reconcile.Dependencies)
		config       func(c *reconcile.Config)
		refused      bool
	}{
		"a reconciler of the roots of the kinds it is given": {},
		"or of none":                                                 {dependencies: func(d *reconcile.Dependencies) { d.Roots = nil }},
		"but not of a kind that is no graph's":                       {dependencies: func(d *reconcile.Dependencies) { d.Roots = []string{taskKind.Name} }, refused: true},
		"nor of one whose resources are never roots":                 {dependencies: func(d *reconcile.Dependencies) { d.Kinds[containerKind.Name] = plain{d.Kinds[containerKind.Name]} }, refused: true},
		"nor of graphs of a kind the control plane does not run":     {dependencies: func(d *reconcile.Dependencies) { d.Kinds["image"] = d.Kinds[vmKind.Name] }, refused: true},
		"nor as no control plane":                                    {dependencies: func(d *reconcile.Dependencies) { d.Holder = "" }, refused: true},
		"nor with nothing to keep its leases in":                     {dependencies: func(d *reconcile.Dependencies) { d.Leases = nil }, refused: true},
		"nor with nothing to read the records from":                  {dependencies: func(d *reconcile.Dependencies) { d.Resources = nil }, refused: true},
		"nor with no worker":                                         {config: func(c *reconcile.Config) { c.Workers = 0 }, refused: true},
		"nor with a lease too short to be renewed a third at a time": {config: func(c *reconcile.Config) { c.Lease = 2 * time.Millisecond }, refused: true},
		"nor backing off for less than nothing":                      {config: func(c *reconcile.Config) { c.Backoff.First = -time.Second }, refused: true},
		"nor reading nothing at a time":                              {config: func(c *reconcile.Config) { c.Batch = 0 }, refused: true},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			config := reconcile.DefaultConfig()
			if tt.config != nil {
				tt.config(&config)
			}

			r, err := reconcile.New(dependencies(t, tt.dependencies), config)
			if tt.refused {
				assert.Error(t, err)
				assert.Nil(t, r)

				return
			}

			require.NoError(t, err)
			assert.NotNil(t, r)
		})
	}

	t.Run("by default, it goes about it as the reconcile loop backs off: 15 s doubling up to 15 min, four workers, a lease of 30 s", func(t *testing.T) {
		t.Parallel()

		assert.Equal(t, reconcile.Config{
			Workers: 4,
			Lease:   30 * time.Second,
			Backoff: graph.Backoff{First: 15 * time.Second, Most: 15 * time.Minute},
			Batch:   100,
		}, reconcile.DefaultConfig())
	})
}

func TestReconciler_Reconcile(t *testing.T) {
	t.Parallel()

	ctx := context.Background()

	t.Run("a vm deleted under a live root is made again first, and then what needs it, in order", func(t *testing.T) {
		t.Parallel()

		kinds := container()
		kinds.Hold(vm, network, volume, web)

		f := newFixture(t, kinds, root())
		r := f.reconciler(t, "one", reconcile.DefaultConfig(), containerKind.Name)

		require.NoError(t, r.Reconcile(ctx, refOf(web)))
		require.Empty(t, kinds.Asked(graphtest.Create), "all of it is there")

		// the vm is deleted, and what lived in it with it: their records
		// are kept, and none of them is made.
		kinds.Record(vm, network, volume, web)

		require.NoError(t, r.Reconcile(ctx, refOf(web)))

		made := kinds.Asked(graphtest.Create)
		require.Len(t, made, 4, "each made once")
		assert.Equal(t, vm, made[0], "the vm first")
		assert.ElementsMatch(t, []graph.NodeID{network, volume}, made[1:3], "then what needs nothing but the vm")
		assert.Equal(t, web, made[3], "and then the root")

		// what needs a node begins to be made only once the node is.
		began, returned := order(kinds.Events(), graphtest.Create)
		for _, edge := range []graph.Edge{{From: vm, To: network}, {From: vm, To: volume}, {From: network, To: web}, {From: volume, To: web}} {
			assert.Less(t, returned[edge.From], began[edge.To], "%s is made before %s is begun", edge.From, edge.To)
		}

		lease := f.lease(t, web)
		assert.Empty(t, lease.Holder, "its lease let go of")
		assert.Zero(t, lease.Failures)

		for _, e := range f.kept(t, web) {
			assert.Equal(t, graph.StateDone, e.State, "%s", e.Node)
			assert.Equal(t, graph.ActionCreate, e.Action, "%s", e.Node)
		}
	})

	t.Run("a healthy root asks for nothing, and writes no record, but its lease and how its plan went", func(t *testing.T) {
		t.Parallel()

		kinds := container()
		kinds.Hold(vm, network, volume, web)

		// the records of its graph, linked as the default policy has them:
		// its network owned by it, its vm and its volume used.
		f := newFixture(t, kinds,
			record(refOf(vm), nil, nil),
			record(refOf(network), []graph.Ref{refOf(vm)}, []graph.Ref{refOf(web)}),
			record(refOf(volume), []graph.Ref{refOf(vm)}, nil),
			root(func(m *kind.Metadata) { m.Refs = []graph.Ref{refOf(vm), refOf(volume)} }),
		)

		// what owns what is what the records say, as it is served.
		f.ownership = ownership.New(f.registry, f.resources, f.clock.Now)

		r := f.reconciler(t, "one", reconcile.DefaultConfig(), containerKind.Name)

		for range 2 {
			require.NoError(t, r.Reconcile(ctx, refOf(web)))
		}

		assert.Empty(t, kinds.Asked(graphtest.Create), "nothing is made")
		assert.Empty(t, kinds.Asked(graphtest.Delete), "nothing is removed")
		assert.Empty(t, f.resources.Writes(), "no record is written")

		lease := f.lease(t, web)
		assert.Empty(t, lease.Holder)
		assert.Zero(t, lease.Failures)

		assert.Equal(t, map[graph.NodeID]graph.State{vm: graph.StateDone, network: graph.StateDone, volume: graph.StateDone, web: graph.StateDone}, states(f.kept(t, web)))
	})

	t.Run("a root asked to be deleted is removed, and never made again, however many reconcile it at once", func(t *testing.T) {
		t.Parallel()

		// its network is missing, which applying it would make.
		kinds := container()
		kinds.Hold(vm, volume, web)

		// removed slowly enough for the others to come while it is.
		kinds.On(graphtest.Delete, web, func(context.Context) error {
			time.Sleep(20 * time.Millisecond)

			return nil
		})

		f := newFixture(t, kinds, root(deleting))

		// once what it owned is removed, its record goes, as the control
		// plane's own does.
		kinds.On(graphtest.Forget, web, func(ctx context.Context) error {
			return f.records.Delete(ctx, containerKind.Name, "web")
		})

		reconcilers := []*reconcile.Reconciler{
			f.reconciler(t, "one", reconcile.DefaultConfig(), containerKind.Name),
			f.reconciler(t, "another", reconcile.DefaultConfig(), containerKind.Name),
		}

		var reconciling sync.WaitGroup
		for i := range 8 {
			reconciling.Go(func() {
				_ = reconcilers[i%2].Reconcile(ctx, refOf(web))
			})
		}

		reconciling.Wait()

		// and each once more, after.
		for _, r := range reconcilers {
			require.NoError(t, r.Reconcile(ctx, refOf(web)))
		}

		assert.Empty(t, kinds.Asked(graphtest.Create), "nothing is made")
		assert.Equal(t, []graph.NodeID{web}, kinds.Asked(graphtest.Delete), "the root is removed, once")
		assert.Equal(t, []graph.NodeID{web}, kinds.Asked(graphtest.Forget))

		require.NotEmpty(t, f.executions.Saved())
		for _, e := range f.executions.Saved() {
			assert.Equal(t, graph.ActionDelete, e.Action, "%s is only ever removed", e.Node)
		}

		_, kept := f.leases.Lease("web")
		assert.False(t, kept, "its lease is forgotten")
		assert.Empty(t, f.kept(t, web), "and so is how its plan went")
	})

	t.Run("a root is carried out at its record's generation, one kept before generations at its first", func(t *testing.T) {
		t.Parallel()

		api := graph.ID(containerKind.Name, "api")

		kinds := container()
		kinds.Require(api, requirement(vm))
		kinds.Hold(vm, network, volume, web, api)

		f := newFixture(t, kinds,
			root(func(m *kind.Metadata) { m.Generation = 3 }),
			record(refOf(api), []graph.Ref{refOf(vm)}, nil, func(m *kind.Metadata) { m.Generation = 0 }),
		)

		r := f.reconciler(t, "one", reconcile.DefaultConfig(), containerKind.Name)

		require.NoError(t, r.Reconcile(ctx, refOf(web)))
		require.NoError(t, r.Reconcile(ctx, refOf(api)))

		for _, e := range f.kept(t, web) {
			assert.Equal(t, int64(3), e.Generation, "%s", e.Node)
		}

		for _, e := range f.kept(t, api) {
			assert.Equal(t, int64(1), e.Generation, "%s", e.Node)
		}
	})

	t.Run("a root that is gone has its lease and how its plan went forgotten, and nothing is asked of its kind", func(t *testing.T) {
		t.Parallel()

		kinds := container()
		f := newFixture(t, kinds)

		// what a run of it left: how its plan went, and its lease, a failure
		// that backs it off for nothing on it.
		require.NoError(t, f.executions.Save(ctx, graph.Execution{Root: refOf(web), Node: web, Kind: containerKind.Name, Action: graph.ActionCreate, State: graph.StateFailed}))

		_, err := f.leases.Take(ctx, "web", "another", time.Minute)
		require.NoError(t, err)

		_, err = f.leases.Release(ctx, "web", "another", graph.Ended{State: graph.StateFailed, Reason: "broken"})
		require.NoError(t, err)

		r := f.reconciler(t, "one", reconcile.DefaultConfig(), containerKind.Name)

		require.NoError(t, r.Reconcile(ctx, refOf(web)))

		_, kept := f.leases.Lease("web")
		assert.False(t, kept, "its lease is forgotten")
		assert.Empty(t, f.kept(t, web), "and how its plan went")
		assert.Empty(t, kinds.Calls(), "and nothing is asked of its kind")
	})

	t.Run("and so does one that went while it was backed off, which nothing would take again to forget", func(t *testing.T) {
		t.Parallel()

		kinds := container()
		f := newFixture(t, kinds)

		_, err := f.leases.Take(ctx, "web", "another", time.Minute)
		require.NoError(t, err)

		_, err = f.leases.Release(ctx, "web", "another", graph.Ended{State: graph.StateFailed, Reason: "broken", Backoff: reconcile.DefaultConfig().Backoff})
		require.NoError(t, err)

		r := f.reconciler(t, "one", reconcile.DefaultConfig(), containerKind.Name)

		require.NoError(t, r.Reconcile(ctx, refOf(web)))

		_, kept := f.leases.Lease("web")
		assert.False(t, kept)
		assert.Empty(t, kinds.Calls())
	})

	t.Run("one backed off that is there is left until its backoff is over", func(t *testing.T) {
		t.Parallel()

		kinds := container()
		f := newFixture(t, kinds, root())

		_, err := f.leases.Take(ctx, "web", "another", time.Minute)
		require.NoError(t, err)

		backedOff, err := f.leases.Release(ctx, "web", "another", graph.Ended{State: graph.StateFailed, Reason: "broken", Backoff: reconcile.DefaultConfig().Backoff})
		require.NoError(t, err)

		r := f.reconciler(t, "one", reconcile.DefaultConfig(), containerKind.Name)

		assert.ErrorIs(t, r.Reconcile(ctx, refOf(web)), graph.ErrBackedOff)
		assert.Equal(t, backedOff, f.lease(t, web), "its lease as it was")
		assert.Empty(t, kinds.Calls())
	})

	t.Run("what something owns now is a root no more: its lease is forgotten, and its graph is not carried out", func(t *testing.T) {
		t.Parallel()

		kinds := container()
		f := newFixture(t, kinds, root(func(m *kind.Metadata) {
			m.Owners = []graph.Ref{{Kind: stackKind.Name, UUID: "s-uuid"}}
		}))

		r := f.reconciler(t, "one", reconcile.DefaultConfig(), containerKind.Name)

		require.NoError(t, r.Reconcile(ctx, refOf(web)))

		_, kept := f.leases.Lease("web")
		assert.False(t, kept)
		assert.Empty(t, kinds.Calls())
	})

	t.Run("but the vm a record kept before refs has among its owners owns nothing, and it is a root all the same", func(t *testing.T) {
		t.Parallel()

		kinds := container()
		kinds.Hold(vm, network, volume, web)

		f := newFixture(t, kinds, record(refOf(web), nil, []graph.Ref{refOf(vm)}))
		r := f.reconciler(t, "one", reconcile.DefaultConfig(), containerKind.Name)

		require.NoError(t, r.Reconcile(ctx, refOf(web)))

		assert.Contains(t, kinds.Asked(graphtest.Exists), web, "its graph is carried out")
		assert.Empty(t, f.lease(t, web).Holder)
	})

	t.Run("what is not of a kind of root here is not reconciled", func(t *testing.T) {
		t.Parallel()

		kinds := container()
		f := newFixture(t, kinds, in(refOf(network)))
		r := f.reconciler(t, "one", reconcile.DefaultConfig(), containerKind.Name)

		assert.ErrorIs(t, r.Reconcile(ctx, refOf(network)), kind.ErrUnknownKind)

		_, kept := f.leases.Lease(network.Name())
		assert.False(t, kept, "no lease is taken")
		assert.Empty(t, kinds.Calls())
		assert.Zero(t, f.resources.Reads())
	})
}

func TestReconciler_Reconcile_lease(t *testing.T) {
	t.Parallel()

	ctx := context.Background()

	t.Run("while one reconciler carries a root out, under its lease, another skips the root", func(t *testing.T) {
		t.Parallel()

		gate := newGate()

		kinds := container()
		kinds.Hold(vm, network, volume)
		kinds.On(graphtest.Create, web, gate.wait)

		f := newFixture(t, kinds, root())

		one := f.reconciler(t, "one", reconcile.DefaultConfig(), containerKind.Name)
		another := f.reconciler(t, "another", reconcile.DefaultConfig(), containerKind.Name)

		done := make(chan error, 1)
		go func() { done <- one.Reconcile(ctx, refOf(web)) }()

		require.Eventually(t, func() bool { return slices.Contains(kinds.Asked(graphtest.Create), web) }, settle, tick, "one is making the root")

		asked := len(kinds.Calls())

		assert.ErrorIs(t, another.Reconcile(ctx, refOf(web)), graph.ErrHeld)
		assert.Len(t, kinds.Calls(), asked, "another asks nothing of it")
		assert.Equal(t, "one", f.lease(t, web).Holder)

		gate.Open()
		require.NoError(t, <-done)

		// let go of, another takes it, and finds it made.
		require.NoError(t, another.Reconcile(ctx, refOf(web)))
		assert.Equal(t, []graph.NodeID{web}, kinds.Asked(graphtest.Create), "made once")
	})

	t.Run("a reconciler asked to reconcile a root it is reconciling skips it, though the lease is its own", func(t *testing.T) {
		t.Parallel()

		gate := newGate()

		kinds := container()
		kinds.Hold(vm, network, volume)
		kinds.On(graphtest.Create, web, gate.wait)

		f := newFixture(t, kinds, root())
		one := f.reconciler(t, "one", reconcile.DefaultConfig(), containerKind.Name)

		done := make(chan error, 1)
		go func() { done <- one.Reconcile(ctx, refOf(web)) }()

		require.Eventually(t, func() bool { return slices.Contains(kinds.Asked(graphtest.Create), web) }, settle, tick)

		asked := len(kinds.Calls())

		assert.ErrorIs(t, one.Reconcile(ctx, refOf(web)), reconcile.ErrReconciling)
		assert.Len(t, kinds.Calls(), asked, "nothing is asked of it")

		gate.Open()
		require.NoError(t, <-done)

		require.NoError(t, one.Reconcile(ctx, refOf(web)), "and it is reconciled once it is done")
	})

	t.Run("its lease is renewed every third of its time while its plan runs, so that it does not lapse under it", func(t *testing.T) {
		t.Parallel()

		config := reconcile.DefaultConfig()
		config.Lease = 30 * time.Millisecond

		gate := newGate()

		kinds := container()
		kinds.Hold(vm, network, volume)
		kinds.On(graphtest.Create, web, gate.wait)

		f := newFixture(t, kinds, root())

		one := f.reconciler(t, "one", config, containerKind.Name)
		another := f.reconciler(t, "another", config, containerKind.Name)

		done := make(chan error, 1)
		go func() { done <- one.Reconcile(ctx, refOf(web)) }()

		require.Eventually(t, func() bool { return slices.Contains(kinds.Asked(graphtest.Create), web) }, settle, tick)

		expires := func(at time.Time) func() bool {
			return func() bool {
				lease, _ := f.leases.Lease("web")

				return lease.ExpiresAt.Equal(at)
			}
		}

		require.Eventually(t, expires(kindstest.Moment.Add(30*time.Millisecond)), settle, tick, "taken until its time from then")

		f.clock.Advance(20 * time.Millisecond)
		require.Eventually(t, expires(kindstest.Moment.Add(50*time.Millisecond)), settle, tick, "renewed until its time from now")

		// past what it was taken until.
		f.clock.Advance(20 * time.Millisecond)

		assert.ErrorIs(t, another.Reconcile(ctx, refOf(web)), graph.ErrHeld)

		gate.Open()
		require.NoError(t, <-done)
	})

	t.Run("a lease lost while its plan runs, which another control plane took once it lapsed, stops the run", func(t *testing.T) {
		t.Parallel()

		config := reconcile.DefaultConfig()
		config.Lease = 30 * time.Millisecond

		gate := newGate()

		kinds := container()
		kinds.Hold(vm, network, volume)
		kinds.On(graphtest.Create, web, gate.wait)

		f := newFixture(t, kinds, root())
		one := f.reconciler(t, "one", config, containerKind.Name)

		done := make(chan error, 1)
		go func() { done <- one.Reconcile(ctx, refOf(web)) }()

		require.Eventually(t, func() bool { return slices.Contains(kinds.Asked(graphtest.Create), web) }, settle, tick)

		// lapsed, and taken by another control plane.
		f.clock.Advance(time.Minute)

		_, err := f.leases.Take(ctx, "web", "another", time.Minute)
		require.NoError(t, err)

		select {
		case err = <-done:
		case <-time.After(settle):
			require.Fail(t, "the run never stopped")
		}

		require.ErrorIs(t, err, graph.ErrLost, "how it ended is not kept on what it does not hold")
		assert.ErrorContains(t, err, "cut short")

		lease := f.lease(t, web)
		assert.Equal(t, "another", lease.Holder, "the lease is another's")
		assert.Zero(t, lease.Failures)
	})
}

func TestReconciler_Reconcile_backoff(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	broken := errors.New("broken")

	t.Run("failures in a row back a root off longer each time, up to the most; one that waits leaves that, and one done clears it", func(t *testing.T) {
		t.Parallel()

		kinds := container()
		kinds.Hold(vm, volume, web)
		kinds.On(graphtest.Create, network, func(context.Context) error { return broken })

		f := newFixture(t, kinds, root())
		r := f.reconciler(t, "one", reconcile.DefaultConfig(), containerKind.Name)

		// the reconcile loop's own numbers.
		for failures, backoff := range []time.Duration{15 * time.Second, 30 * time.Second, time.Minute, 2 * time.Minute, 4 * time.Minute, 8 * time.Minute, 15 * time.Minute, 15 * time.Minute} {
			require.ErrorIs(t, r.Reconcile(ctx, refOf(web)), broken)

			lease := f.lease(t, web)
			assert.Empty(t, lease.Holder)
			assert.Equal(t, failures+1, lease.Failures)
			assert.Equal(t, f.clock.Now().Add(backoff), lease.NextAt, "backed off for %s after %d failures", backoff, failures+1)
			assert.Contains(t, lease.Reason, "broken")

			asked := len(kinds.Calls())

			require.ErrorIs(t, r.Reconcile(ctx, refOf(web)), graph.ErrBackedOff, "until then")
			assert.Len(t, kinds.Calls(), asked, "nothing is asked of it")

			f.clock.Advance(backoff)
		}

		kinds.On(graphtest.Create, network, func(context.Context) error {
			return &graph.Waiting{Reason: "its vm is stopped"}
		})

		before := f.lease(t, web)

		require.NoError(t, r.Reconcile(ctx, refOf(web)), "a run that waits did not fail")

		waited := f.lease(t, web)
		assert.Equal(t, before.Failures, waited.Failures, "and adds no failure")
		assert.Equal(t, before.NextAt, waited.NextAt, "nor backs it off")
		assert.Equal(t, before.Reason, waited.Reason)

		kinds.On(graphtest.Create, network, nil)

		require.NoError(t, r.Reconcile(ctx, refOf(web)))

		done := f.lease(t, web)
		assert.Zero(t, done.Failures, "a run done clears them")
		assert.True(t, done.NextAt.IsZero())
		assert.Empty(t, done.Reason)
	})

	t.Run("a graph that cannot be planned is a failure, saying why", func(t *testing.T) {
		t.Parallel()

		// what it needs is made after each other.
		kinds := graphtest.NewKind()
		kinds.Require(web, requirement(network, volume), requirement(volume, network))

		f := newFixture(t, kinds, root())
		r := f.reconciler(t, "one", reconcile.DefaultConfig(), containerKind.Name)

		err := r.Reconcile(ctx, refOf(web))

		var cycle *graph.Cycle
		require.ErrorAs(t, err, &cycle)

		lease := f.lease(t, web)
		assert.Equal(t, 1, lease.Failures)
		assert.Contains(t, lease.Reason, "depends_on cycle")
		assert.Equal(t, f.clock.Now().Add(15*time.Second), lease.NextAt)
		assert.Empty(t, kinds.Asked(graphtest.Exists), "nothing of it is carried out")
	})

	t.Run("and so is a record that does not say what its root needs", func(t *testing.T) {
		t.Parallel()

		kinds := container()
		kinds.On(graphtest.Root, web, func(context.Context) error { return broken })

		f := newFixture(t, kinds, root())
		r := f.reconciler(t, "one", reconcile.DefaultConfig(), containerKind.Name)

		require.ErrorIs(t, r.Reconcile(ctx, refOf(web)), broken)

		lease := f.lease(t, web)
		assert.Equal(t, 1, lease.Failures)
		assert.Contains(t, lease.Reason, "what the root needs cannot be read off its record")
	})

	t.Run("and so is a run cut short, which is taken up once its backoff is over", func(t *testing.T) {
		t.Parallel()

		gate := newGate()

		kinds := container()
		kinds.Hold(vm, network, volume)
		kinds.On(graphtest.Create, web, gate.wait)

		f := newFixture(t, kinds, root())
		r := f.reconciler(t, "one", reconcile.DefaultConfig(), containerKind.Name)

		stopping, stop := context.WithCancel(ctx)

		done := make(chan error, 1)
		go func() { done <- r.Reconcile(stopping, refOf(web)) }()

		require.Eventually(t, func() bool { return slices.Contains(kinds.Asked(graphtest.Create), web) }, settle, tick)

		stop()

		err := <-done
		assert.ErrorContains(t, err, "cut short")

		lease := f.lease(t, web)
		assert.Empty(t, lease.Holder, "let go of, though the run was cancelled")
		assert.Equal(t, 1, lease.Failures)
		assert.Contains(t, lease.Reason, "cut short")

		assert.Equal(t, graph.StatePending, states(f.kept(t, web))[web], "what it cut short is not done")

		gate.Open()
		f.clock.Advance(15 * time.Second)

		require.NoError(t, r.Reconcile(ctx, refOf(web)))
		assert.Equal(t, []graph.NodeID{web, web}, kinds.Asked(graphtest.Create), "and is taken up")
		assert.Zero(t, f.lease(t, web).Failures)
	})
}

// order is where, among events, each node was first asked op, and where it
// last returned from it.
func order(events []graphtest.Event, op graphtest.Op) (map[graph.NodeID]int, map[graph.NodeID]int) {
	began := make(map[graph.NodeID]int)
	returned := make(map[graph.NodeID]int)

	for i, event := range events {
		switch {
		case event.Op != op:
		case event.Returned:
			returned[event.Node] = i
		default:
			if _, seen := began[event.Node]; !seen {
				began[event.Node] = i
			}
		}
	}

	return began, returned
}

// states are the state each node went to, by node.
func states(executions []graph.Execution) map[graph.NodeID]graph.State {
	nodes := make(map[graph.NodeID]graph.State, len(executions))
	for _, e := range executions {
		nodes[e.Node] = e.State
	}

	return nodes
}
