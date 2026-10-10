package execute_test

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/khanzadimahdi/testproject/application/workload/controlplane/graph/execute"
	"github.com/khanzadimahdi/testproject/application/workload/controlplane/kinds/kindstest"
	"github.com/khanzadimahdi/testproject/domain/workload/graph"
	"github.com/khanzadimahdi/testproject/domain/workload/graph/graphtest"
	"github.com/khanzadimahdi/testproject/domain/workload/kind"
	containerKind "github.com/khanzadimahdi/testproject/domain/workload/kinds/container"
	networkKind "github.com/khanzadimahdi/testproject/domain/workload/kinds/network"
	vmKind "github.com/khanzadimahdi/testproject/domain/workload/kinds/vm"
	executionsMemory "github.com/khanzadimahdi/testproject/infrastructure/repository/memory/workload/executions"
)

func TestExecutor_Apply(t *testing.T) {
	t.Parallel()

	broken := errors.New("broken")

	t.Run("a plan is made from the bottom up: every node of a level returns before the next level is asked anything", func(t *testing.T) {
		t.Parallel()

		kinds := services(map[graph.NodeID][]graph.NodeID{web: {api}, api: {db}})
		for _, id := range []graph.NodeID{vm, network, volume, db, api, web, stack} {
			// long enough a create for a level started too soon to show.
			kinds.On(graphtest.Create, id, func(context.Context) error {
				time.Sleep(5 * time.Millisecond)

				return nil
			})
		}

		f := newFixture(t, kinds)
		plan := f.plan(t, stack)

		levels := plan.Levels()
		require.Equal(t, [][]graph.NodeID{{vm}, {network, volume}, {db}, {api}, {web}, {stack}}, levels)

		outcome := f.apply(t, plan)
		require.Equal(t, execute.Done, outcome.State)

		// where each node was first looked for, and where it last returned.
		began := make(map[graph.NodeID]int)
		returned := make(map[graph.NodeID]int)

		for i, event := range kinds.Events() {
			switch {
			case event.Op != graphtest.Exists && event.Op != graphtest.Create:
			case event.Returned:
				returned[event.Node] = i
			default:
				if _, seen := began[event.Node]; !seen {
					began[event.Node] = i
				}
			}
		}

		for i := 1; i < len(levels); i++ {
			for _, before := range levels[i-1] {
				for _, after := range levels[i] {
					assert.Less(t, returned[before], began[after], "%s is made before %s is looked for", before, after)
				}
			}
		}

		assert.ElementsMatch(t, []graph.NodeID{vm, network, volume, db, api, web, stack}, kinds.Asked(graphtest.Create), "every node made once")
	})

	t.Run("a plan made already is made again by nothing: every node is found, and done", func(t *testing.T) {
		t.Parallel()

		f := newFixture(t, container(false))
		plan := f.plan(t, web)

		require.Equal(t, execute.Done, f.apply(t, plan).State)

		made := f.kinds.Asked(graphtest.Create)
		require.Len(t, made, 4)

		again := f.apply(t, plan)
		assert.Equal(t, execute.Done, again.State)
		assert.NoError(t, again.Err)

		assert.Equal(t, made, f.kinds.Asked(graphtest.Create), "no node is made again")
		assert.Len(t, f.kinds.Asked(graphtest.Exists), 8, "each is looked for again")

		for _, e := range again.Nodes {
			assert.Equal(t, graph.StateDone, e.State, "%s", e.Node)
			assert.Equal(t, graphtest.Ref(e.Node), e.Ref, "%s as the record it was found to be", e.Node)
		}

		assert.Equal(t, again.Nodes, f.kept(t, plan), "and is kept so")
	})

	t.Run("a plan that failed part way is taken up where it stopped, and only what was not made is made", func(t *testing.T) {
		t.Parallel()

		kinds := container(false)
		kinds.On(graphtest.Create, volume, func(context.Context) error { return broken })

		f := newFixture(t, kinds)
		plan := f.plan(t, web)

		failed := f.apply(t, plan)
		assert.Equal(t, execute.Failed, failed.State)
		require.ErrorIs(t, failed.Err, broken)
		assert.EqualError(t, failed.Err, "volume/s_data: broken")

		assert.NotContains(t, kinds.Asked(graphtest.Exists), web, "the level after it is never started")

		stored := byNode(f.kept(t, plan))
		assert.Equal(t, graph.StateFailed, stored[volume].State)
		assert.Equal(t, "broken", stored[volume].Reason)
		assert.Equal(t, graph.StateDone, stored[network].State, "what was made beside it stays made")
		assert.Equal(t, graph.StatePending, stored[web].State)

		kinds.On(graphtest.Create, volume, nil)

		made := len(kinds.Asked(graphtest.Create))
		looked := len(kinds.Asked(graphtest.Exists))

		resumed := f.apply(t, plan)
		assert.Equal(t, execute.Done, resumed.State)
		assert.NoError(t, resumed.Err)

		assert.Equal(t, []graph.NodeID{volume, web}, kinds.Asked(graphtest.Create)[made:], "only what was not made is made")
		assert.ElementsMatch(t, []graph.NodeID{vm, network, volume, web}, kinds.Asked(graphtest.Exists)[looked:], "and what was is found")

		assert.Equal(t, map[graph.NodeID]graph.State{
			vm:      graph.StateDone,
			network: graph.StateDone,
			volume:  graph.StateDone,
			web:     graph.StateDone,
		}, states(f.kept(t, plan)))
	})

	t.Run("a node that waits stops the plan as waiting, not as failed", func(t *testing.T) {
		t.Parallel()

		kinds := container(false)
		kinds.On(graphtest.Create, network, func(context.Context) error {
			return fmt.Errorf("the network: %w", &graph.Waiting{Reason: "its vm is stopped"})
		})

		f := newFixture(t, kinds)
		plan := f.plan(t, web)

		outcome := f.apply(t, plan)
		assert.Equal(t, execute.Waiting, outcome.State)
		assert.NoError(t, outcome.Err, "nothing failed")

		waits, _ := outcome.Node(network)
		assert.Equal(t, graph.StateWaiting, waits.State)
		assert.Equal(t, "its vm is stopped", waits.Reason, "found however it was wrapped")

		assert.Equal(t, map[graph.NodeID]graph.State{
			vm:      graph.StateDone,
			network: graph.StateWaiting,
			volume:  graph.StateDone,
			web:     graph.StatePending,
		}, states(f.kept(t, plan)))

		assert.NotContains(t, kinds.Asked(graphtest.Exists), web, "the level after it is never started")
	})

	t.Run("an external node that is missing is waited on, and never made", func(t *testing.T) {
		t.Parallel()

		kinds := container(true)

		f := newFixture(t, kinds)
		plan := f.plan(t, web)

		mounted, _ := plan.Node(volume)
		require.True(t, mounted.External)

		outcome := f.apply(t, plan)
		assert.Equal(t, execute.Waiting, outcome.State)
		assert.NoError(t, outcome.Err)

		waits, _ := outcome.Node(volume)
		assert.Equal(t, graph.StateWaiting, waits.State)
		assert.Equal(t, "it is external, and missing", waits.Reason)
		assert.Equal(t, waits, byNode(f.kept(t, plan))[volume], "and is kept so")

		assert.Contains(t, kinds.Asked(graphtest.Exists), volume, "it is looked for")
		assert.NotContains(t, kinds.Asked(graphtest.Create), volume, "and never made")
		assert.NotContains(t, kinds.Asked(graphtest.Exists), web, "and what needs it is not started")
	})

	t.Run("and one that is there is done, as the record it is", func(t *testing.T) {
		t.Parallel()

		kinds := container(true)
		kinds.Hold(volume)

		f := newFixture(t, kinds)
		plan := f.plan(t, web)

		outcome := f.apply(t, plan)
		assert.Equal(t, execute.Done, outcome.State)

		there, _ := outcome.Node(volume)
		assert.Equal(t, graph.StateDone, there.State)
		assert.Equal(t, graphtest.Ref(volume), there.Ref)

		assert.NotContains(t, kinds.Asked(graphtest.Create), volume, "it is not made")
		assert.Contains(t, kinds.Asked(graphtest.Create), web, "while what needs it is")
	})

	t.Run("each create is handed the records of what it needs in the plan, and nothing else", func(t *testing.T) {
		t.Parallel()

		// web is made after api, which it does not otherwise need, and the
		// vm is there already.
		kinds := services(map[graph.NodeID][]graph.NodeID{web: {api}})
		kinds.Hold(vm)

		f := newFixture(t, kinds)
		require.Equal(t, execute.Done, f.apply(t, f.plan(t, stack)).State)

		ref := graphtest.Ref

		handed := make(map[graph.NodeID]map[graph.NodeID]graph.Ref)
		for _, call := range kinds.Calls() {
			if call.Op == graphtest.Create {
				handed[call.Node] = call.Deps
			}
		}

		assert.Equal(t, map[graph.NodeID]map[graph.NodeID]graph.Ref{
			network: {vm: ref(vm)},
			volume:  {vm: ref(vm)},
			db:      {vm: ref(vm), network: ref(network), volume: ref(volume)},
			api:     {vm: ref(vm), network: ref(network)},
			web:     {vm: ref(vm), network: ref(network), api: ref(api)},
			stack:   {web: ref(web), api: ref(api), db: ref(db)},
		}, handed, "the vm, found, is handed on as made ones are, and is made by none")
	})

	t.Run("how each node goes is saved as it goes: its state, its reason, its record and the clock's times", func(t *testing.T) {
		t.Parallel()

		back := graph.ID(networkKind.Name, "s_back")

		// one at a time, each a minute after it was started: web's vm is
		// there, its back network waits, its network is made, and its volume
		// fails.
		kinds := graphtest.NewKind()
		kinds.Require(web, requirement(vm), requirement(back), requirement(network), requirement(volume))
		kinds.Require(back, requirement(vm))
		kinds.Require(network, requirement(vm))
		kinds.Require(volume, requirement(vm))
		kinds.Hold(vm)

		f := newFixture(t, kinds, execute.Concurrency(1))

		later := func(err error) func(context.Context) error {
			return func(context.Context) error {
				f.clock.Advance(time.Minute)

				return err
			}
		}

		kinds.On(graphtest.Exists, vm, later(nil))
		kinds.On(graphtest.Create, back, later(&graph.Waiting{Reason: "its vm is stopped"}))
		kinds.On(graphtest.Create, network, later(nil))
		kinds.On(graphtest.Create, volume, later(broken))

		root := graph.Ref{Kind: containerKind.Name, UUID: "web-uuid"}
		plan := f.plan(t, web)

		outcome, err := f.executor.Apply(context.Background(), root, 7, plan)
		require.NoError(t, err)

		at := func(minutes int) time.Time {
			return kindstest.Moment.Add(time.Duration(minutes) * time.Minute)
		}

		pending := func(id graph.NodeID, level int) graph.Execution {
			return graph.Execution{
				Root:       root,
				Generation: 7,
				Node:       id,
				Kind:       id.Kind(),
				Action:     graph.ActionCreate,
				Level:      level,
				State:      graph.StatePending,
				UpdatedAt:  at(0),
			}
		}

		running := func(id graph.NodeID, level int, started int) graph.Execution {
			e := pending(id, level)
			e.State = graph.StateRunning
			e.StartedAt = at(started)
			e.UpdatedAt = at(started)

			return e
		}

		ended := func(e graph.Execution, state graph.State, finished int, reason string, ref graph.Ref) graph.Execution {
			e.State = state
			e.Reason = reason
			e.Ref = ref
			e.FinishedAt = at(finished)
			e.UpdatedAt = at(finished)

			return e
		}

		assert.Equal(t, []graph.Execution{
			pending(vm, 0),
			pending(back, 1),
			pending(network, 1),
			pending(volume, 1),
			pending(web, 2),
			running(vm, 0, 0),
			ended(running(vm, 0, 0), graph.StateDone, 1, "", graphtest.Ref(vm)),
			running(back, 1, 1),
			ended(running(back, 1, 1), graph.StateWaiting, 2, "its vm is stopped", graph.Ref{}),
			running(network, 1, 2),
			ended(running(network, 1, 2), graph.StateDone, 3, "", graphtest.Ref(network)),
			running(volume, 1, 3),
			ended(running(volume, 1, 3), graph.StateFailed, 4, "broken", graph.Ref{}),
		}, f.executions.Saved(), "every node pending first, and then each as it went, web never started")

		want := []graph.Execution{
			ended(running(vm, 0, 0), graph.StateDone, 1, "", graphtest.Ref(vm)),
			ended(running(back, 1, 1), graph.StateWaiting, 2, "its vm is stopped", graph.Ref{}),
			ended(running(network, 1, 2), graph.StateDone, 3, "", graphtest.Ref(network)),
			ended(running(volume, 1, 3), graph.StateFailed, 4, "broken", graph.Ref{}),
			pending(web, 2),
		}

		assert.Equal(t, execute.Failed, outcome.State, "a node failed, whatever else waits")
		require.ErrorIs(t, outcome.Err, broken)
		assert.Equal(t, want, outcome.Nodes, "level by level, each in the order of its ids")

		kept, err := f.executions.GetByRoot(context.Background(), root.UUID)
		require.NoError(t, err)
		assert.Equal(t, want, kept)
	})

	t.Run("a plan with a node of a kind the executor is not given is carried out not at all", func(t *testing.T) {
		t.Parallel()

		ctx := context.Background()
		kinds := container(false)

		plan, err := graph.NewPlanner(kinds.Kinds(every...), graph.DefaultPolicy()).Plan(requirement(web))
		require.NoError(t, err)

		executions := &recorder{Repository: executionsMemory.NewRepository()}

		executor, err := execute.New(kinds.Kinds(vmKind.Name, networkKind.Name, containerKind.Name), executions, nil)
		require.NoError(t, err)

		_, err = executor.Apply(ctx, graphtest.Ref(web), 1, plan)
		require.ErrorIs(t, err, kind.ErrUnknownKind)
		assert.EqualError(t, err, `unknown kind: "volume", the kind of volume/s_data`)

		assert.Empty(t, executions.Saved(), "nothing of it is saved")
		assert.Empty(t, kinds.Asked(graphtest.Exists), "nor looked for")

		_, err = executor.Apply(ctx, graphtest.Ref(web), 1, nil)
		assert.Error(t, err, "and no plan is none to carry out")
	})
}

func TestExecutor_Apply_concurrency(t *testing.T) {
	t.Parallel()

	for name, tt := range map[string]struct {
		services int
		options  []execute.Option
		together int
	}{
		"a level of four is made all at once, when four may be": {services: 4, options: []execute.Option{execute.Concurrency(4)}, together: 4},
		"never more than two at once, when two may be":          {services: 4, options: []execute.Option{execute.Concurrency(2)}, together: 2},
		"one at a time, when one may be":                        {services: 4, options: []execute.Option{execute.Concurrency(1)}, together: 1},
		"and four at a time, unless the executor is told":       {services: 6, together: 4},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			// a stack of services that need nothing: a level of them, and
			// then the stack.
			kinds := graphtest.NewKind()
			flight := newFlight(tt.together)

			var required []graph.Requirement
			for i := range tt.services {
				id := graph.ID(containerKind.Name, fmt.Sprintf("s_%d", i))
				required = append(required, requirement(id))

				kinds.On(graphtest.Create, id, flight.create)
			}

			kinds.Require(stack, required...)

			f := newFixture(t, kinds, tt.options...)

			outcome := f.apply(t, f.plan(t, stack))
			require.Equal(t, execute.Done, outcome.State, "%v", outcome.Err)

			assert.Equal(t, tt.together, flight.most(), "as many at once as may be, and never more")
		})
	}
}

func TestExecutor_Apply_cancelled(t *testing.T) {
	t.Parallel()

	t.Run("a run cancelled starts nothing more, waits for what runs, and saves it pending again, as interrupted", func(t *testing.T) {
		t.Parallel()

		// one at a time: the network is made until the run is cancelled, and
		// the volume, after it in its level, waits for its turn until then.
		kinds := container(false)

		started := make(chan struct{})
		kinds.On(graphtest.Create, network, func(ctx context.Context) error {
			close(started)
			<-ctx.Done()

			return ctx.Err()
		})

		f := newFixture(t, kinds, execute.Concurrency(1))
		plan := f.plan(t, web)

		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		type applied struct {
			outcome execute.Outcome
			err     error
		}

		over := make(chan applied, 1)

		go func() {
			outcome, err := f.executor.Apply(ctx, graphtest.Ref(web), 1, plan)
			over <- applied{outcome: outcome, err: err}
		}()

		select {
		case <-started:
		case <-time.After(10 * time.Second):
			require.FailNow(t, "the network was never made")
		}

		cancel()

		var result applied
		select {
		case result = <-over:
		case <-time.After(10 * time.Second):
			require.FailNow(t, "the run never ended")
		}

		require.NoError(t, result.err)
		assert.Equal(t, execute.Interrupted, result.outcome.State)
		assert.NoError(t, result.outcome.Err, "nothing failed")

		stored := byNode(f.kept(t, plan))

		assert.Equal(t, graph.StateDone, stored[vm].State, "what was made before stays made")

		assert.Equal(t, graph.StatePending, stored[network].State, "what ran is pending again, and did not fail")
		assert.Equal(t, "interrupted before it was done: context canceled", stored[network].Reason)
		assert.True(t, stored[network].StartedAt.IsZero(), "as it was before it was started")
		assert.True(t, stored[network].FinishedAt.IsZero())

		assert.Equal(t, graph.StatePending, stored[volume].State, "what never started is as it was")
		assert.Empty(t, stored[volume].Reason)
		assert.Equal(t, graph.StatePending, stored[web].State)

		assert.NotContains(t, kinds.Asked(graphtest.Exists), volume, "and nothing more is started")
		assert.NotContains(t, kinds.Asked(graphtest.Exists), web)

		assert.Equal(t, result.outcome.Nodes, f.kept(t, plan), "the run returned once what ran was saved")
	})

	t.Run("one cancelled before it began starts nothing", func(t *testing.T) {
		t.Parallel()

		kinds := container(false)

		f := newFixture(t, kinds)
		plan := f.plan(t, web)

		ctx, cancel := context.WithCancel(context.Background())
		cancel()

		outcome, err := f.executor.Apply(ctx, graphtest.Ref(web), 1, plan)
		require.NoError(t, err)
		assert.Equal(t, execute.Interrupted, outcome.State)

		assert.Empty(t, kinds.Asked(graphtest.Exists))
		assert.Equal(t, map[graph.NodeID]graph.State{
			vm:      graph.StatePending,
			network: graph.StatePending,
			volume:  graph.StatePending,
			web:     graph.StatePending,
		}, states(f.kept(t, plan)))
	})
}

func TestExecutor_Apply_unsaved(t *testing.T) {
	t.Parallel()

	gone := errors.New("the database is gone")

	t.Run("a node not saved running is not started, and the run's error is returned once what runs beside it has ended", func(t *testing.T) {
		t.Parallel()

		// the network, beside the volume, takes a moment to make.
		kinds := container(false)
		kinds.On(graphtest.Create, network, func(context.Context) error {
			time.Sleep(20 * time.Millisecond)

			return nil
		})

		f := newFixture(t, kinds)
		f.executions.failWith(func(e graph.Execution) error {
			if e.Node == volume && e.State == graph.StateRunning {
				return gone
			}

			return nil
		})

		plan := f.plan(t, web)

		outcome, err := f.executor.Apply(context.Background(), graphtest.Ref(web), 1, plan)
		require.ErrorIs(t, err, gone)
		assert.EqualError(t, err, "how volume/s_data went cannot be saved: the database is gone")

		stored := byNode(f.kept(t, plan))
		assert.Equal(t, graph.StateDone, stored[network].State, "the network ended, and was saved, before the run returned")
		assert.Equal(t, graph.StatePending, stored[volume].State)

		assert.NotContains(t, kinds.Asked(graphtest.Exists), volume, "the volume is not started")
		assert.NotContains(t, kinds.Asked(graphtest.Exists), web, "nor anything after it")

		assert.Equal(t, execute.Interrupted, outcome.State)
		assert.Equal(t, map[graph.NodeID]graph.State{
			vm:      graph.StateDone,
			network: graph.StateDone,
			volume:  graph.StatePending,
			web:     graph.StatePending,
		}, states(outcome.Nodes), "how far it went")
	})

	t.Run("a plan whose nodes cannot be saved pending is not started", func(t *testing.T) {
		t.Parallel()

		kinds := container(false)

		f := newFixture(t, kinds)
		f.executions.failWith(func(graph.Execution) error { return gone })

		_, err := f.executor.Apply(context.Background(), graphtest.Ref(web), 1, f.plan(t, web))
		require.ErrorIs(t, err, gone)

		assert.Empty(t, kinds.Asked(graphtest.Exists))
	})
}

// flight is how many creates are under way at once, and the most that ever
// were. The first together of them wait for one another, so that as many as
// that are under way at once when the executor lets them be, and fail once
// they have waited long enough when it does not; and each is under way a
// moment longer, so that any the executor should not have let start beside
// them would be under way with them.
type flight struct {
	lock     sync.Mutex
	together int
	arrived  int
	now      int
	highest  int
	all      chan struct{}
}

func newFlight(together int) *flight {
	return &flight{together: together, all: make(chan struct{})}
}

// create is one create under way.
func (f *flight) create(ctx context.Context) error {
	f.lock.Lock()
	f.now++
	f.highest = max(f.highest, f.now)
	f.arrived++

	first := f.arrived <= f.together
	if f.arrived == f.together {
		close(f.all)
	}
	f.lock.Unlock()

	defer func() {
		f.lock.Lock()
		defer f.lock.Unlock()

		f.now--
	}()

	if first {
		select {
		case <-f.all:
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(10 * time.Second):
			return fmt.Errorf("%d creates were never under way at once", f.together)
		}
	}

	time.Sleep(20 * time.Millisecond)

	return nil
}

// most is the most creates that were under way at once.
func (f *flight) most() int {
	f.lock.Lock()
	defer f.lock.Unlock()

	return f.highest
}
