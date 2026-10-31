package execute_test

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"sync"
	"sync/atomic"
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
	volumeKind "github.com/khanzadimahdi/testproject/domain/workload/kinds/volume"
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

	t.Run("a node whose record is there and not made is made, as one with none is, and one made is not", func(t *testing.T) {
		t.Parallel()

		// the network's record is there, being made say, and the vm is
		// made.
		kinds := container(false)
		kinds.Record(network)
		kinds.Hold(vm)

		f := newFixture(t, kinds)
		plan := f.plan(t, web)

		outcome := f.apply(t, plan)
		require.Equal(t, execute.Done, outcome.State)

		assert.ElementsMatch(t, []graph.NodeID{network, volume, web}, kinds.Asked(graphtest.Create), "made only what is not made, whatever its record")

		made, _ := outcome.Node(network)
		assert.Equal(t, graphtest.Ref(network), made.Ref, "as the record it was made")
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

	t.Run("and so is one its kind says is held with nobody keeping a record of it, as it is held, while nothing that is not external is looked for so", func(t *testing.T) {
		t.Parallel()

		ctx := context.Background()
		kinds := container(true)

		f := newFixture(t, kinds)
		plan := f.plan(t, web)

		// the volume its vm's dockerd holds, made from its terminal.
		seenInItsVM := graph.Ref{Kind: volumeKind.Name, UUID: "seen-in-its-vm"}
		holder := &holding{Kind: kinds, held: map[graph.NodeID]graph.Ref{volume: seenInItsVM, network: {Kind: networkKind.Name, UUID: "seen-too"}}}

		held := kinds.Kinds(every...)
		held[volumeKind.Name] = holder
		held[networkKind.Name] = holder

		executor, err := execute.New(held, f.ownership, f.executions, f.clock.Now)
		require.NoError(t, err)

		previewed, err := executor.Preview(ctx, plan)
		require.NoError(t, err)

		for _, level := range previewed.Levels {
			for _, node := range level {
				if node.Node == volume {
					assert.Equal(t, execute.Exists, node.Verdict, "a preview finds it as a run does")
					assert.Equal(t, seenInItsVM, node.Ref)
				}
			}
		}

		outcome, err := executor.Apply(ctx, graphtest.Ref(web), 1, plan)
		require.NoError(t, err)
		assert.Equal(t, execute.Done, outcome.State)

		there, _ := outcome.Node(volume)
		assert.Equal(t, graph.StateDone, there.State)
		assert.Equal(t, seenInItsVM, there.Ref, "as what holds it")

		assert.NotContains(t, kinds.Asked(graphtest.Create), volume, "it is not made")
		assert.Contains(t, kinds.Asked(graphtest.Create), network, "and the network, which is not external, is made, however its kind finds one")
		assert.Equal(t, []graph.NodeID{volume, volume}, holder.Asked(), "only what is external is looked for where it is held")
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

		executor, err := execute.New(kinds.Kinds(vmKind.Name, networkKind.Name, containerKind.Name), kinds.Ownership(), executions, nil)
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

// TestExecutor_Apply_again holds a run of a plan carried out before, every
// node of which is kept at its generation, to saving nothing while it finds
// each node as the run before left it, and to saving all a run saves, from
// its start, once it finds one otherwise or is to make one.
func TestExecutor_Apply_again(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	gone := errors.New("the database is gone")

	// applied is a fixture whose plan of web was carried out once, a minute
	// before its clock says now, and what that run saved.
	applied := func(t *testing.T, kinds *graphtest.Kind, options ...execute.Option) (*fixture, *graph.Plan, []graph.Execution) {
		t.Helper()

		f := newFixture(t, kinds, options...)
		plan := f.plan(t, web)

		_, err := f.executor.Apply(ctx, graphtest.Ref(web), 1, plan)
		require.NoError(t, err)

		f.clock.Advance(time.Minute)

		return f, plan, f.executions.Saved()
	}

	t.Run("one that finds every node as the run before left it saves nothing, and what is kept is how that run went", func(t *testing.T) {
		t.Parallel()

		f, plan, saved := applied(t, container(false))
		before := f.kept(t, plan)

		again := f.apply(t, plan)
		assert.Equal(t, execute.Done, again.State)

		assert.Equal(t, saved, f.executions.Saved(), "nothing is saved")
		assert.Equal(t, before, f.kept(t, plan), "its times among it")
		assert.Len(t, f.kinds.Asked(graphtest.Exists), 8, "though each node is looked for again")
	})

	t.Run("and so does one that waits where the run before waited, on what is not its own and missing", func(t *testing.T) {
		t.Parallel()

		f, plan, saved := applied(t, container(true))
		before := f.kept(t, plan)

		again := f.apply(t, plan)
		assert.Equal(t, execute.Waiting, again.State)

		assert.Equal(t, saved, f.executions.Saved())
		assert.Equal(t, before, f.kept(t, plan))
	})

	t.Run("one cut short saves nothing", func(t *testing.T) {
		t.Parallel()

		kinds := container(false)
		f, plan, saved := applied(t, kinds)
		before := f.kept(t, plan)

		cutShort, cancel := context.WithCancel(ctx)
		defer cancel()

		kinds.On(graphtest.Exists, network, func(ctx context.Context) error {
			cancel()

			return ctx.Err()
		})

		outcome, err := f.executor.Apply(cutShort, graphtest.Ref(web), 1, plan)
		require.NoError(t, err)
		assert.Equal(t, execute.Interrupted, outcome.State)

		assert.Equal(t, saved, f.executions.Saved())
		assert.Equal(t, before, f.kept(t, plan))
	})

	t.Run("and so does one cut short before it reads what is kept, which it reads all the same", func(t *testing.T) {
		t.Parallel()

		kinds := container(false)
		f, plan, saved := applied(t, kinds)
		before := f.kept(t, plan)

		// what is kept is read as MongoDB reads it: not at all once the
		// read's context is done.
		executor, err := execute.New(kinds.Kinds(every...), f.ownership, honouring{f.executions}, f.clock.Now)
		require.NoError(t, err)

		cutShort, cancel := context.WithCancel(ctx)
		cancel()

		outcome, err := executor.Apply(cutShort, graphtest.Ref(web), 1, plan)
		require.NoError(t, err)
		assert.Equal(t, execute.Interrupted, outcome.State)

		assert.Equal(t, saved, f.executions.Saved(), "every node pending is not saved over the plan the run before carried out")
		assert.Equal(t, before, f.kept(t, plan))
	})

	t.Run("one that finds a node to make saves all a run saves from its start, and makes it once that is saved", func(t *testing.T) {
		t.Parallel()

		kinds := container(false)
		f, plan, saved := applied(t, kinds, execute.Concurrency(1))

		// its record is there, and it is not made.
		kinds.Record(network)

		again := f.apply(t, plan)
		assert.Equal(t, execute.Done, again.State)

		type went struct {
			Node  graph.NodeID
			State graph.State
		}

		var said []went
		for _, e := range f.executions.Saved()[len(saved):] {
			said = append(said, went{Node: e.Node, State: e.State})
		}

		assert.Equal(t, []went{
			{vm, graph.StatePending}, {network, graph.StatePending}, {volume, graph.StatePending}, {web, graph.StatePending},
			{vm, graph.StateRunning}, {vm, graph.StateDone},
			{network, graph.StateRunning}, {network, graph.StateDone},
			{volume, graph.StateRunning}, {volume, graph.StateDone},
			{web, graph.StateRunning}, {web, graph.StateDone},
		}, said, "every node pending first, and then each as it went, as a run saves them")

		assert.Equal(t, again.Nodes, f.kept(t, plan), "kept as this run went")
	})

	t.Run("and one that finds a node ended otherwise does too", func(t *testing.T) {
		t.Parallel()

		kinds := container(false)
		f, plan, saved := applied(t, kinds)

		kinds.On(graphtest.Exists, volume, func(context.Context) error { return gone })

		again := f.apply(t, plan)
		assert.Equal(t, execute.Failed, again.State)

		assert.Greater(t, len(f.executions.Saved()), len(saved))
		assert.Equal(t, again.Nodes, f.kept(t, plan))
	})

	t.Run("and so does one at another generation", func(t *testing.T) {
		t.Parallel()

		f, plan, saved := applied(t, container(false))

		_, err := f.executor.Apply(ctx, graphtest.Ref(web), 2, plan)
		require.NoError(t, err)

		assert.Len(t, f.executions.Saved(), 2*len(saved), "what the first run saved, again")

		for _, e := range f.kept(t, plan) {
			assert.Equal(t, int64(2), e.Generation, "%s", e.Node)
		}
	})

	t.Run("one that cannot save what it held before it makes a node makes nothing, and is what saving failed with", func(t *testing.T) {
		t.Parallel()

		kinds := container(false)
		f, plan, _ := applied(t, kinds)
		made := len(kinds.Asked(graphtest.Create))

		kinds.Record(network)
		f.executions.failWith(func(e graph.Execution) error {
			if e.Node == network && e.State == graph.StateRunning {
				return gone
			}

			return nil
		})

		outcome, err := f.executor.Apply(ctx, graphtest.Ref(web), 1, plan)
		require.ErrorIs(t, err, gone)

		assert.Len(t, kinds.Asked(graphtest.Create), made, "the network is not made")
		assert.Equal(t, execute.Interrupted, outcome.State)
		assert.Equal(t, graph.StatePending, byNode(outcome.Nodes)[network].State)
	})

	t.Run("and one that saves only part of what it held puts back what it saved of it as the run before left it", func(t *testing.T) {
		t.Parallel()

		kinds := container(false)
		f, plan, _ := applied(t, kinds, execute.Concurrency(1))
		before := f.kept(t, plan)

		// its network is to be made, and of what the run held as it was
		// quiet, the vm and the network pending are saved, and the volume
		// pending is not.
		kinds.Record(network)
		f.executions.failWith(func(e graph.Execution) error {
			if e.Node == volume && e.State == graph.StatePending {
				return gone
			}

			return nil
		})

		_, err := f.executor.Apply(ctx, graphtest.Ref(web), 1, plan)
		require.ErrorIs(t, err, gone)

		assert.Equal(t, before, f.kept(t, plan), "what the plan says is how the run before went, and not the vm pending, which this run found done, beside it")
	})

	t.Run("and so is the one whose save failed, which may have been carried out all the same", func(t *testing.T) {
		t.Parallel()

		kinds := container(false)
		f, plan, _ := applied(t, kinds, execute.Concurrency(1))
		before := f.kept(t, plan)

		// the volume pending is kept, and its save fails all the same: a
		// write whose answer timed out after it was carried out.
		kinds.Record(network)
		f.executions.failOnceKept(func(e graph.Execution) error {
			if e.Node == volume && e.State == graph.StatePending {
				return gone
			}

			return nil
		})

		_, err := f.executor.Apply(ctx, graphtest.Ref(web), 1, plan)
		require.ErrorIs(t, err, gone)

		assert.Equal(t, before, f.kept(t, plan), "what the plan says is how the run before went, the volume among it")
	})
}

// honouring reads what is kept as MongoDB does: not at all once the read's
// context is done.
type honouring struct {
	*recorder
}

func (h honouring) GetByRoot(ctx context.Context, rootUUID string) ([]graph.Execution, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	return h.recorder.GetByRoot(ctx, rootUUID)
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

				kinds.On(graphtest.Create, id, flight.call)
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

// flight is how many calls, creates or deletes, are under way at once, and
// the most that ever were. The first together of them wait for one another,
// so that as many as that are under way at once when the executor lets them
// be, and fail once they have waited long enough when it does not; and each
// is under way a moment longer, so that any the executor should not have let
// start beside them would be under way with them.
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

// call is one call under way.
func (f *flight) call(ctx context.Context) error {
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
			return fmt.Errorf("%d calls were never under way at once", f.together)
		}
	}

	time.Sleep(20 * time.Millisecond)

	return nil
}

// most is the most calls that were under way at once.
func (f *flight) most() int {
	f.lock.Lock()
	defer f.lock.Unlock()

	return f.highest
}

func TestExecutor_Apply_links(t *testing.T) {
	t.Parallel()

	ref := graphtest.Ref

	// links are the links a run asked for, in order, by the node linked.
	links := func(calls []graphtest.Call) map[graph.NodeID][]graphtest.Call {
		linked := make(map[graph.NodeID][]graphtest.Call)
		for _, call := range calls {
			if call.Op == graphtest.Link {
				linked[call.Node] = append(linked[call.Node], call)
			}
		}

		return linked
	}

	link := func(id graph.NodeID, needed graph.NodeID, t graph.EdgeType) graphtest.Call {
		return graphtest.Call{Op: graphtest.Link, Node: id, Needed: ref(needed), Type: t}
	}

	t.Run("a node there is linked to each node it needs as the policy types the edge, once it is made", func(t *testing.T) {
		t.Parallel()

		f := newFixture(t, container(false))
		require.Equal(t, execute.Done, f.apply(t, f.plan(t, web)).State)

		assert.Equal(t, map[graph.NodeID][]graphtest.Call{
			network: {link(network, vm, graph.Loose)},
			volume:  {link(volume, vm, graph.Loose)},
			web:     {link(web, network, graph.Owned), link(web, vm, graph.Loose), link(web, volume, graph.Loose)},
		}, links(f.kinds.Calls()), "in the order of what it needs, and nothing for the vm, which needs nothing")

		// where each node was made, and where it was first linked.
		made := make(map[graph.NodeID]int)
		linked := make(map[graph.NodeID]int)

		for i, event := range f.kinds.Events() {
			switch {
			case event.Op == graphtest.Create && event.Returned:
				made[event.Node] = i
			case event.Op == graphtest.Link && !event.Returned:
				if _, seen := linked[event.Node]; !seen {
					linked[event.Node] = i
				}
			}
		}

		for _, id := range []graph.NodeID{network, volume, web} {
			assert.Less(t, made[id], linked[id], "%s is made before it is linked", id)
		}

		assert.Equal(t, []graph.NodeID{web}, f.ownership.OwnersOf(network), "web owns its network")
		assert.Equal(t, []graph.NodeID{vm, volume}, f.ownership.RefsOf(web), "and uses its vm and its volume")
		assert.Empty(t, f.ownership.OwnersOf(volume))
	})

	t.Run("and one found there as well, so that a plan applied again is linked again", func(t *testing.T) {
		t.Parallel()

		f := newFixture(t, container(false))
		plan := f.plan(t, web)

		require.Equal(t, execute.Done, f.apply(t, plan).State)
		first := links(f.kinds.Calls())

		asked := len(f.kinds.Calls())

		require.Equal(t, execute.Done, f.apply(t, plan).State)
		assert.Equal(t, first, links(f.kinds.Calls()[asked:]), "every node, though nothing is made")
		assert.Len(t, f.kinds.Asked(graphtest.Create), 4)
	})

	t.Run("a service is linked to one it is only made after as an order, which holds nothing", func(t *testing.T) {
		t.Parallel()

		kinds := services(map[graph.NodeID][]graph.NodeID{web: {api}})

		f := newFixture(t, kinds)
		require.Equal(t, execute.Done, f.apply(t, f.plan(t, stack)).State)

		assert.Contains(t, links(kinds.Calls())[web], link(web, api, graph.Order))
		assert.Equal(t, []graph.NodeID{stack}, f.ownership.OwnersOf(api), "api is owned by its stack, and by nothing it is ordered before")
	})

	t.Run("nothing external is owned: what needs it, and what it needs, are linked to it as loose", func(t *testing.T) {
		t.Parallel()

		kinds := container(true)
		kinds.Hold(volume)

		f := newFixture(t, kinds)
		require.Equal(t, execute.Done, f.apply(t, f.planUnder(t, web, "volume=owned,vm=owned")).State)

		linked := links(kinds.Calls())

		assert.Contains(t, linked[web], link(web, volume, graph.Loose), "whatever the policy says of a volume")
		assert.Contains(t, linked[volume], link(volume, vm, graph.Loose), "and of what the volume needs")
		assert.Contains(t, linked[web], link(web, vm, graph.Owned), "while what is not external is owned as it says")
		assert.Contains(t, linked[network], link(network, vm, graph.Owned))

		assert.Empty(t, f.ownership.OwnersOf(volume))
		assert.Equal(t, []graph.NodeID{web, network}, f.ownership.OwnersOf(vm), "in the order of their ids")
	})

	t.Run("a policy changed between two runs is what the records say once the second is done", func(t *testing.T) {
		t.Parallel()

		f := newFixture(t, container(false))

		// what lives in a vm uses it, whatever owns what.
		for _, kindName := range []string{networkKind.Name, volumeKind.Name, containerKind.Name} {
			f.ownership.Parent(kindName, vmKind.Name)
		}

		for _, run := range []struct {
			policy string
			owners map[graph.NodeID][]graph.NodeID
			refs   map[graph.NodeID][]graph.NodeID
		}{
			{
				policy: "",
				owners: map[graph.NodeID][]graph.NodeID{network: {web}},
				refs:   map[graph.NodeID][]graph.NodeID{network: {vm}, volume: {vm}, web: {vm, volume}},
			},
			{
				// owned to loose takes web out of what owns its network, and
				// loose to owned its volume out of what web uses.
				policy: "network=loose,volume=owned",
				owners: map[graph.NodeID][]graph.NodeID{volume: {web}},
				refs:   map[graph.NodeID][]graph.NodeID{network: {vm}, volume: {vm}, web: {network, vm}},
			},
			{
				// what lives in a vm owns it now, and uses it still.
				policy: "vm=owned",
				owners: map[graph.NodeID][]graph.NodeID{vm: {web, network, volume}, network: {web}},
				refs:   map[graph.NodeID][]graph.NodeID{network: {vm}, volume: {vm}, web: {vm, volume}},
			},
		} {
			require.Equal(t, execute.Done, f.apply(t, f.planUnder(t, web, run.policy)).State, run.policy)

			for _, id := range []graph.NodeID{vm, network, volume, web} {
				assert.Equal(t, run.owners[id], f.ownership.OwnersOf(id), "%q: what owns %s", run.policy, id)
				assert.Equal(t, run.refs[id], f.ownership.RefsOf(id), "%q: what %s uses", run.policy, id)
			}
		}

		assert.Len(t, f.kinds.Asked(graphtest.Create), 4, "made once, and linked each time")
	})

	t.Run("a link that fails fails its node, saying so, and the next run links it", func(t *testing.T) {
		t.Parallel()

		broken := errors.New("broken")

		kinds := container(false)
		kinds.On(graphtest.Link, web, func(context.Context) error { return broken })

		f := newFixture(t, kinds)
		plan := f.plan(t, web)

		failed := f.apply(t, plan)
		assert.Equal(t, execute.Failed, failed.State)
		require.ErrorIs(t, failed.Err, broken)

		e, _ := failed.Node(web)
		assert.Equal(t, graph.StateFailed, e.State)
		assert.Equal(t, "it is there, and network/s_default, which it needs, cannot be linked to it as owned: broken", e.Reason)
		assert.Contains(t, kinds.Asked(graphtest.Create), web, "it was made")
		assert.Empty(t, f.ownership.OwnersOf(network), "and is linked to nothing")

		kinds.On(graphtest.Link, web, nil)

		again := f.apply(t, plan)
		assert.Equal(t, execute.Done, again.State)
		assert.Equal(t, []graph.NodeID{web}, f.ownership.OwnersOf(network))
		assert.Equal(t, []graph.NodeID{web}, kinds.Asked(graphtest.Create)[3:], "and is not made again")
	})
}

// TestExecutor_Apply_while holds a run to making what is not there only while
// what it is for wants it made: once that says otherwise, what is there is
// done as it is, and what is not is never made, and ends as it says.
func TestExecutor_Apply_while(t *testing.T) {
	t.Parallel()

	ctx := context.Background()

	t.Run("what is not there is made while it is wanted, asked once for each", func(t *testing.T) {
		t.Parallel()

		f := newFixture(t, container(false))
		plan := f.plan(t, web)

		var asked atomic.Int64

		outcome, err := f.executor.Apply(ctx, graphtest.Ref(web), 1, plan, execute.While(func(context.Context) error {
			asked.Add(1)

			return nil
		}))
		require.NoError(t, err)

		assert.Equal(t, execute.Done, outcome.State)
		assert.Equal(t, int64(len(plan.Nodes())), asked.Load(), "once for each node made")
	})

	t.Run("and once it is not, nothing more is made, and what was to be ends as it says", func(t *testing.T) {
		t.Parallel()

		kinds := container(false)
		kinds.Hold(vm)

		f := newFixture(t, kinds)

		outcome, err := f.executor.Apply(ctx, graphtest.Ref(web), 1, f.plan(t, web), execute.While(func(context.Context) error {
			return &graph.Waiting{Reason: "its root was asked to go"}
		}))
		require.NoError(t, err)

		assert.Equal(t, execute.Waiting, outcome.State)
		assert.Empty(t, kinds.Asked(graphtest.Create), "nothing is made")

		there, _ := outcome.Node(vm)
		assert.Equal(t, graph.StateDone, there.State, "what is there is done as it is")

		waiting, _ := outcome.Node(network)
		assert.Equal(t, graph.StateWaiting, waiting.State)
		assert.Equal(t, "its root was asked to go", waiting.Reason)
	})
}

// TestExecutor_Apply_generations holds a run of a newer generation of a
// root's plan to forgetting what is kept of a plan before that its own no
// longer has, once it has saved every node of its own, so that what is kept
// is the plan as it is: and to forgetting nothing until then, so that a run
// that saved only part of its plan leaves the rest as the run before left
// it.
func TestExecutor_Apply_generations(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	gone := errors.New("the database is gone")

	// dropped is a node web's plan of its first generation had, and its
	// plan now does not.
	dropped := graph.ID(networkKind.Name, "s_dropped")

	// first is a fixture whose plan of web was carried out at its first
	// generation, when it had the dropped network too.
	first := func(t *testing.T) (*fixture, *graph.Plan) {
		t.Helper()

		f := newFixture(t, container(false), execute.Concurrency(1))
		plan := f.plan(t, web)

		_, err := f.executor.Apply(ctx, graphtest.Ref(web), 1, plan)
		require.NoError(t, err)

		require.NoError(t, f.executions.Save(ctx, graph.Execution{Root: graphtest.Ref(web), Generation: 1, Node: dropped, Kind: networkKind.Name, Action: graph.ActionCreate, Level: 1, State: graph.StateDone}))

		return f, plan
	}

	nodes := func(t *testing.T, f *fixture) map[graph.NodeID]int64 {
		t.Helper()

		kept, err := f.executions.GetByRoot(ctx, graphtest.Ref(web).UUID)
		require.NoError(t, err)

		generations := make(map[graph.NodeID]int64, len(kept))
		for _, e := range kept {
			generations[e.Node] = e.Generation
		}

		return generations
	}

	t.Run("a run of a newer generation forgets the node its plan no longer has, once it saved every node of its own", func(t *testing.T) {
		t.Parallel()

		f, plan := first(t)

		_, err := f.executor.Apply(ctx, graphtest.Ref(web), 2, plan)
		require.NoError(t, err)

		assert.Equal(t, map[graph.NodeID]int64{vm: 2, network: 2, volume: 2, web: 2}, nodes(t, f))
	})

	t.Run("and so does one that finds every node of its plan as the run before left it, saving nothing else", func(t *testing.T) {
		t.Parallel()

		f, plan := first(t)

		_, err := f.executor.Apply(ctx, graphtest.Ref(web), 2, plan)
		require.NoError(t, err)

		// left by a run that could not forget it.
		require.NoError(t, f.executions.Save(ctx, graph.Execution{Root: graphtest.Ref(web), Generation: 1, Node: dropped, Kind: networkKind.Name, Action: graph.ActionCreate, Level: 1, State: graph.StateDone}))
		saved := len(f.executions.Saved())

		_, err = f.executor.Apply(ctx, graphtest.Ref(web), 2, plan)
		require.NoError(t, err)

		assert.Equal(t, map[graph.NodeID]int64{vm: 2, network: 2, volume: 2, web: 2}, nodes(t, f))
		assert.Len(t, f.executions.Saved(), saved, "nothing saved")
	})

	t.Run("but one that could not save every node of its own forgets nothing, the rest as the run before left it", func(t *testing.T) {
		t.Parallel()

		f, plan := first(t)

		f.executions.failWith(func(e graph.Execution) error {
			if e.Node == volume && e.Generation == 2 {
				return gone
			}

			return nil
		})

		_, err := f.executor.Apply(ctx, graphtest.Ref(web), 2, plan)
		require.ErrorIs(t, err, gone)

		assert.Equal(t, map[graph.NodeID]int64{vm: 2, network: 2, volume: 1, web: 1, dropped: 1}, nodes(t, f))
	})

	t.Run("nor does one of the same generation, which has nothing of another kept", func(t *testing.T) {
		t.Parallel()

		f := newFixture(t, container(false))
		plan := f.plan(t, web)

		_, err := f.executor.Apply(ctx, graphtest.Ref(web), 1, plan)
		require.NoError(t, err)

		assert.Equal(t, map[graph.NodeID]int64{vm: 1, network: 1, volume: 1, web: 1}, nodes(t, f))
	})
}

// holding is the kinds a fake kind is, which also says what holds a node it
// keeps no record of (graph.Holder): what a Docker VM's dockerd holds that
// nobody keeps a record of, made from its terminal.
type holding struct {
	*graphtest.Kind

	held map[graph.NodeID]graph.Ref

	lock  sync.Mutex
	asked []graph.NodeID
}

var _ graph.Holder = &holding{}

func (h *holding) Holds(ctx context.Context, spec any) (graph.Ref, bool, error) {
	id, _ := spec.(graph.NodeID)

	h.lock.Lock()
	h.asked = append(h.asked, id)
	h.lock.Unlock()

	ref, there, err := h.Kind.Exists(ctx, spec)
	if err != nil || there || ref != (graph.Ref{}) {
		return ref, there, err
	}

	held, holds := h.held[id]

	return held, holds, nil
}

// Asked are the nodes it was asked what holds, in order.
func (h *holding) Asked() []graph.NodeID {
	h.lock.Lock()
	defer h.lock.Unlock()

	return slices.Clone(h.asked)
}
