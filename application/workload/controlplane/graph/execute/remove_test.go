package execute_test

import (
	"context"
	"errors"
	"fmt"
	"slices"
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

// the nodes beside a plan's own the tests remove: another root, a container
// of its own, and the networks it or a service shares or owns alone.
var (
	other  = graph.ID(containerKind.Name, "c_other")
	shared = graph.ID(networkKind.Name, "shared")
	front  = graph.ID(networkKind.Name, "s_front")
	inner  = graph.ID(networkKind.Name, "s_inner")
)

// chain is web made after api, and api after db.
var chain = map[graph.NodeID][]graph.NodeID{web: {api}, api: {db}}

func TestExecutor_Remove(t *testing.T) {
	t.Parallel()

	broken := errors.New("broken")

	t.Run("a stack goes first, then its services in the reverse of the order they depend_on one another, then its network, each level once the one before it is gone, and its vm and its volume stay", func(t *testing.T) {
		t.Parallel()

		kinds := declared(chain)
		for _, id := range []graph.NodeID{stack, web, api, db, network} {
			// long enough a delete for a level started too soon to show.
			kinds.On(graphtest.Delete, id, func(context.Context) error {
				time.Sleep(5 * time.Millisecond)

				return nil
			})
		}

		f := newFixture(t, kinds)
		plan := f.plan(t, stack)
		f.made(plan)

		outcome := f.remove(t, plan)
		require.Equal(t, execute.Done, outcome.State)
		assert.NoError(t, outcome.Err)

		assert.Equal(t, [][]graph.NodeID{{stack}, {web}, {api}, {db}, {network}}, byLevel(outcome.Nodes))
		assert.Equal(t, []graph.NodeID{stack, web, api, db, network}, kinds.Asked(graphtest.Delete), "each once")

		began, returned := deletes(kinds)

		removed := kinds.Asked(graphtest.Delete)
		for i := 1; i < len(removed); i++ {
			assert.Less(t, returned[removed[i-1]], began[removed[i]], "%s is gone before %s is removed", removed[i-1], removed[i])
		}

		assert.Equal(t, []graph.NodeID{vm, volume}, kinds.Held(), "the vm and the volume stay")

		for _, call := range kinds.Calls() {
			if call.Op != graphtest.Requires {
				assert.NotContains(t, []graph.NodeID{vm, volume}, call.Node, "and are asked nothing: %s", call.Op)
			}
		}

		for _, e := range outcome.Nodes {
			assert.Equal(t, graph.StateDone, e.State, "%s", e.Node)
			assert.Equal(t, graph.ActionDelete, e.Action, "%s", e.Node)
			assert.Equal(t, graphtest.Ref(e.Node), e.Ref, "%s as the record it was", e.Node)
			assert.Empty(t, e.Reason, "%s", e.Node)
		}
	})

	t.Run("a volume its stack owns goes at a level after the service that mounts it loosely, once that is gone, and never beside it", func(t *testing.T) {
		t.Parallel()

		// the stack s of web, on its network, and db, which joins none and
		// mounts the stack's volume: the stack owns its volume, as the pair
		// says, and db uses it loosely, as every container does a volume.
		kinds := graphtest.NewKind()
		kinds.Require(stack, requirement(web), requirement(db), requirement(network), requirement(volume))
		kinds.Require(web, requirement(vm), requirement(network))
		kinds.Require(db, requirement(vm), requirement(volume))
		kinds.Require(network, requirement(vm))
		kinds.Require(volume, requirement(vm))

		// long enough a delete of db for the volume's, were it started
		// beside it, to begin before it returned.
		kinds.On(graphtest.Delete, db, func(context.Context) error {
			time.Sleep(20 * time.Millisecond)

			return nil
		})

		f := newFixture(t, kinds)
		plan := f.planUnder(t, stack, "stack/volume=owned")
		f.made(plan)

		for edge, want := range map[[2]graph.NodeID]graph.EdgeType{
			{volume, stack}: graph.Owned,
			{volume, db}:    graph.Loose,
		} {
			e, found := plan.Edge(edge[0], edge[1])
			require.True(t, found, "%s is made before %s", edge[0], edge[1])
			require.Equal(t, want, e.Type, "%s to %s", edge[0], edge[1])
		}

		outcome := f.remove(t, plan)
		require.Equal(t, execute.Done, outcome.State)

		assert.Equal(t, [][]graph.NodeID{{stack}, {db, web}, {network, volume}}, byLevel(outcome.Nodes))

		mounting, _ := outcome.Node(db)
		mounted, _ := outcome.Node(volume)
		assert.Greater(t, mounted.Level, mounting.Level, "the volume is removed at a level after db's")

		began, returned := deletes(kinds)
		assert.Less(t, returned[db], began[volume], "db is gone before the volume is asked to go")

		assert.ElementsMatch(t, []graph.NodeID{stack, web, db, network, volume}, kinds.Asked(graphtest.Delete))
		assert.Equal(t, []graph.NodeID{vm}, kinds.Held(), "and only the vm stays")
	})

	t.Run("a network two roots own outlives the delete of one, unlinked rather than removed, and goes with the delete of the other", func(t *testing.T) {
		t.Parallel()

		kinds := graphtest.NewKind()
		kinds.Require(web, requirement(vm), requirement(shared))
		kinds.Require(other, requirement(vm), requirement(shared))
		kinds.Require(shared, requirement(vm))

		f := newFixture(t, kinds)
		plan := f.plan(t, web)
		others := f.plan(t, other)
		f.made(plan, others)

		require.Equal(t, []graph.NodeID{other, web}, f.ownership.OwnersOf(shared))

		outcome := f.remove(t, plan)
		require.Equal(t, execute.Done, outcome.State)

		assert.Equal(t, [][]graph.NodeID{{shared}, {web}}, byLevel(outcome.Nodes), "unlinked before anything is removed")

		kept, _ := outcome.Node(shared)
		assert.Equal(t, graph.StateDone, kept.State)
		assert.Equal(t, "kept, owned by container/c_other", kept.Reason)
		assert.Equal(t, graphtest.Ref(shared), kept.Ref)

		assert.Equal(t, []graph.NodeID{web}, kinds.Asked(graphtest.Delete), "only web is removed")
		assert.Equal(t, []graph.NodeID{other}, f.ownership.OwnersOf(shared), "and the network is the other's alone")
		assert.Equal(t, []graph.NodeID{other, shared, vm}, kinds.Held())

		calls := kinds.Calls()
		assert.Less(t,
			slices.IndexFunc(calls, func(c graphtest.Call) bool { return c.Op == graphtest.Disown && c.Node == shared }),
			slices.IndexFunc(calls, func(c graphtest.Call) bool { return c.Op == graphtest.Delete && c.Node == web }),
			"it is unlinked before web is removed",
		)

		again := f.remove(t, others)
		require.Equal(t, execute.Done, again.State)

		assert.Equal(t, [][]graph.NodeID{{other}, {shared}}, byLevel(again.Nodes))
		assert.Equal(t, []graph.NodeID{web, other, shared}, kinds.Asked(graphtest.Delete), "the network goes with the last of what owns it")
		assert.Equal(t, []graph.NodeID{vm}, kinds.Held())
	})

	t.Run("a node owned by what stays is kept, and so is what it owns, and what that owns, until nothing more is", func(t *testing.T) {
		t.Parallel()

		// the stack's web is another root's too; front is web's alone, and
		// inner front's.
		kinds := graphtest.NewKind()
		kinds.Require(stack, requirement(web), requirement(api), requirement(network))
		kinds.Require(web, requirement(vm), requirement(front))
		kinds.Require(api, requirement(vm), requirement(network))
		kinds.Require(front, requirement(vm), requirement(inner))
		kinds.Require(inner, requirement(vm))
		kinds.Require(network, requirement(vm))

		f := newFixture(t, kinds)
		plan := f.plan(t, stack)
		f.made(plan)

		kinds.Hold(other)
		f.ownership.Own(web, other)

		outcome := f.remove(t, plan)
		require.Equal(t, execute.Done, outcome.State)

		assert.Equal(t, [][]graph.NodeID{{web, front, inner}, {stack}, {api}, {network}}, byLevel(outcome.Nodes))

		for id, reason := range map[graph.NodeID]string{
			web:   "kept, owned by container/c_other",
			front: "kept, owned by container/s_web",
			inner: "kept, owned by network/s_front",
		} {
			kept, _ := outcome.Node(id)
			assert.Equal(t, graph.StateDone, kept.State, "%s", id)
			assert.Equal(t, reason, kept.Reason, "%s", id)
		}

		assert.Equal(t, []graph.NodeID{stack, api, network}, kinds.Asked(graphtest.Delete))

		assert.Equal(t, []graph.NodeID{other}, f.ownership.OwnersOf(web), "web is unlinked from the stack")
		assert.Equal(t, []graph.NodeID{web}, f.ownership.OwnersOf(front), "and what stays still owns what it owns")
		assert.Equal(t, []graph.NodeID{front}, f.ownership.OwnersOf(inner))

		calls := kinds.Calls()
		removed := slices.IndexFunc(calls, func(c graphtest.Call) bool { return c.Op == graphtest.Delete })

		for _, id := range []graph.NodeID{web, front, inner} {
			unlinked := slices.IndexFunc(calls, func(c graphtest.Call) bool { return c.Op == graphtest.Disown && c.Node == id })
			assert.Less(t, unlinked, removed, "%s is unlinked before anything is removed", id)
		}
	})

	t.Run("a node something came to own meanwhile is kept at its turn, unlinked rather than removed, and so is what it owns", func(t *testing.T) {
		t.Parallel()

		kinds := graphtest.NewKind()
		kinds.Require(stack, requirement(web), requirement(api), requirement(network))
		kinds.Require(web, requirement(vm), requirement(front))
		kinds.Require(api, requirement(vm), requirement(network))
		kinds.Require(front, requirement(vm))
		kinds.Require(network, requirement(vm))

		f := newFixture(t, kinds)
		plan := f.plan(t, stack)
		f.made(plan)

		// while the stack is removed, another root comes to own web.
		kinds.On(graphtest.Delete, stack, func(context.Context) error {
			kinds.Hold(other)
			f.ownership.Own(web, other)

			return nil
		})

		outcome := f.remove(t, plan)
		require.Equal(t, execute.Done, outcome.State)

		assert.Equal(t, [][]graph.NodeID{{stack}, {api, web}, {network, front}}, byLevel(outcome.Nodes))

		kept, _ := outcome.Node(web)
		assert.Equal(t, "kept, owned by container/c_other", kept.Reason)

		kept, _ = outcome.Node(front)
		assert.Equal(t, "kept, owned by container/s_web", kept.Reason, "what it owns stays with it")

		assert.ElementsMatch(t, []graph.NodeID{stack, api, network}, kinds.Asked(graphtest.Delete))
		assert.Equal(t, []graph.NodeID{other, web, front, vm}, kinds.Held())
		assert.Equal(t, []graph.NodeID{other}, f.ownership.OwnersOf(web))
		assert.Equal(t, []graph.NodeID{web}, f.ownership.OwnersOf(front))
	})

	t.Run("what is gone already is done with nothing to remove, and a record not made is removed as a made one is", func(t *testing.T) {
		t.Parallel()

		kinds := declared(chain)

		f := newFixture(t, kinds)
		plan := f.plan(t, stack)
		f.made(plan)

		// api went before, and db is being made.
		require.NoError(t, kinds.Delete(context.Background(), graphtest.Ref(api)))
		kinds.Record(db)

		before := len(kinds.Asked(graphtest.Delete))

		outcome := f.remove(t, plan)
		require.Equal(t, execute.Done, outcome.State)

		gone, _ := outcome.Node(api)
		assert.Equal(t, graph.StateDone, gone.State)
		assert.Equal(t, "it is gone already", gone.Reason)
		assert.Zero(t, gone.Ref, "it is no record")

		assert.Equal(t, []graph.NodeID{stack, web, db, network}, kinds.Asked(graphtest.Delete)[before:], "nothing is asked of what is gone, and what is not made goes")
		assert.NotContains(t, kinds.Asked(graphtest.Disown), api)
		assert.Equal(t, []graph.NodeID{vm, volume}, kinds.Held())
	})

	t.Run("a root gone with everything it owns removes nothing, and is forgotten", func(t *testing.T) {
		t.Parallel()

		kinds := declared(nil)
		kinds.Hold(vm, volume)

		f := newFixture(t, kinds)
		plan := f.plan(t, stack)

		outcome := f.remove(t, plan)
		require.Equal(t, execute.Done, outcome.State)

		for _, e := range outcome.Nodes {
			assert.Equal(t, graph.StateDone, e.State, "%s", e.Node)
			assert.Equal(t, "it is gone already", e.Reason, "%s", e.Node)
		}

		assert.Empty(t, kinds.Asked(graphtest.Delete))
		assert.Equal(t, []graph.NodeID{stack}, kinds.Asked(graphtest.Forget))
		assert.Empty(t, f.kept(t, plan))
	})

	t.Run("a node that waits stops the delete as waiting: nothing after its level is started, and the root is not forgotten", func(t *testing.T) {
		t.Parallel()

		kinds := declared(chain)
		kinds.On(graphtest.Delete, web, func(context.Context) error {
			return fmt.Errorf("the container: %w", &graph.Waiting{Reason: "its vm is stopped"})
		})

		f := newFixture(t, kinds)
		plan := f.plan(t, stack)
		f.made(plan)

		outcome := f.remove(t, plan)
		assert.Equal(t, execute.Waiting, outcome.State)
		assert.NoError(t, outcome.Err, "nothing failed")

		waits, _ := outcome.Node(web)
		assert.Equal(t, graph.StateWaiting, waits.State)
		assert.Equal(t, "its vm is stopped", waits.Reason, "found however it was wrapped")
		assert.Equal(t, graphtest.Ref(web), waits.Ref, "the record it is")

		assert.Equal(t, map[graph.NodeID]graph.State{
			stack:   graph.StateDone,
			web:     graph.StateWaiting,
			api:     graph.StatePending,
			db:      graph.StatePending,
			network: graph.StatePending,
		}, states(f.kept(t, plan)), "and kept so")

		assert.Equal(t, []graph.NodeID{stack, web}, kinds.Asked(graphtest.Delete))
		assert.Empty(t, kinds.Asked(graphtest.Forget))

		assert.Equal(t, []graph.NodeID{stack}, f.ownership.OwnersOf(web), "and what waits to be removed is still owned by what removes it, so that it is never taken for a root of its own")
		assert.NotContains(t, kinds.Asked(graphtest.Disown), web, "what nothing else owns is removed as it is")
	})

	t.Run("a node that fails stops the delete as failed, and a delete again is taken up where it stopped", func(t *testing.T) {
		t.Parallel()

		kinds := declared(chain)
		kinds.On(graphtest.Delete, api, func(context.Context) error { return broken })

		f := newFixture(t, kinds)
		plan := f.plan(t, stack)
		f.made(plan)

		failed := f.remove(t, plan)
		assert.Equal(t, execute.Failed, failed.State)
		require.ErrorIs(t, failed.Err, broken)
		assert.EqualError(t, failed.Err, "container/s_api: broken")

		stored := byNode(f.kept(t, plan))
		assert.Equal(t, graph.StateFailed, stored[api].State)
		assert.Equal(t, "broken", stored[api].Reason)
		assert.Equal(t, graph.StateDone, stored[web].State, "what is removed stays removed")
		assert.Equal(t, graph.StatePending, stored[db].State)
		assert.Equal(t, graph.StatePending, stored[network].State)

		assert.Equal(t, []graph.NodeID{stack, web, api}, kinds.Asked(graphtest.Delete))
		assert.Empty(t, kinds.Asked(graphtest.Forget))

		kinds.On(graphtest.Delete, api, nil)

		resumed := f.remove(t, plan)
		require.Equal(t, execute.Done, resumed.State)

		for _, id := range []graph.NodeID{stack, web} {
			gone, _ := resumed.Node(id)
			assert.Equal(t, "it is gone already", gone.Reason, "%s", id)
		}

		assert.Equal(t, []graph.NodeID{api, db, network}, kinds.Asked(graphtest.Delete)[3:], "only what was not removed is removed")
		assert.Equal(t, []graph.NodeID{vm, volume}, kinds.Held())
		assert.Equal(t, []graph.NodeID{stack}, kinds.Asked(graphtest.Forget))
	})

	t.Run("an unlink that fails stops the delete before anything is removed", func(t *testing.T) {
		t.Parallel()

		kinds := graphtest.NewKind()
		kinds.Require(web, requirement(vm), requirement(shared))
		kinds.Require(other, requirement(vm), requirement(shared))
		kinds.Require(shared, requirement(vm))
		kinds.On(graphtest.Disown, shared, func(context.Context) error { return broken })

		f := newFixture(t, kinds)
		plan := f.plan(t, web)
		f.made(plan, f.plan(t, other))

		outcome := f.remove(t, plan)
		assert.Equal(t, execute.Failed, outcome.State)
		assert.EqualError(t, outcome.Err, "network/shared: broken")

		assert.Empty(t, kinds.Asked(graphtest.Delete))
		assert.Equal(t, map[graph.NodeID]graph.State{shared: graph.StateFailed, web: graph.StatePending}, states(f.kept(t, plan)))
		assert.Equal(t, []graph.NodeID{other, web}, f.ownership.OwnersOf(shared), "and nothing is unlinked")
	})

	t.Run("what is loose, external or only ordered is never touched: nothing is removed through a loose edge or an order", func(t *testing.T) {
		t.Parallel()

		// web uses its vm and its volume; owns its network, which is made
		// after the volume for the order alone; and joins front, which is
		// somebody else's.
		kinds := graphtest.NewKind()
		kinds.Require(web, requirement(vm), requirement(network, volume), requirement(volume), external(front))
		kinds.Require(network, requirement(vm))
		kinds.Require(volume, requirement(vm))
		kinds.Require(front, requirement(vm))

		f := newFixture(t, kinds)
		plan := f.plan(t, web)
		f.made(plan)

		for edge, want := range map[[2]graph.NodeID]graph.EdgeType{
			{volume, network}: graph.Order,
			{volume, web}:     graph.Loose,
			{vm, web}:         graph.Loose,
			{front, web}:      graph.Owned,
		} {
			e, found := plan.Edge(edge[0], edge[1])
			require.True(t, found, "%s is made before %s", edge[0], edge[1])
			require.Equal(t, want, e.Type, "%s to %s", edge[0], edge[1])
		}

		outcome := f.remove(t, plan)
		require.Equal(t, execute.Done, outcome.State)

		assert.Equal(t, [][]graph.NodeID{{web}, {network}}, byLevel(outcome.Nodes), "only what web owns is carried out")
		assert.Equal(t, []graph.NodeID{web, network}, kinds.Asked(graphtest.Delete))
		assert.Equal(t, []graph.NodeID{front, vm, volume}, kinds.Held())

		for _, call := range kinds.Calls() {
			if call.Op != graphtest.Requires {
				assert.NotContains(t, []graph.NodeID{vm, volume, front}, call.Node, "nothing is asked of what web does not own: %s", call.Op)
			}
		}
	})

	t.Run("how each node goes is saved as it goes, kept and gone ones too, its action delete, and forgotten once nothing of the root is left", func(t *testing.T) {
		t.Parallel()

		ctx := context.Background()

		// web, a root of its own, owns its network, front, which is gone,
		// and a network another root shares; its vm it uses, and is never
		// asked anything.
		kinds := graphtest.NewKind()
		kinds.Require(web, requirement(vm), requirement(network), requirement(front), requirement(shared))
		kinds.Require(other, requirement(vm), requirement(shared))
		kinds.Require(network, requirement(vm))
		kinds.Require(front, requirement(vm))
		kinds.Require(shared, requirement(vm))

		// one at a time, each a minute after it was started.
		f := newFixture(t, kinds, execute.Concurrency(1))
		plan := f.plan(t, web)
		f.made(plan, f.plan(t, other))

		require.NoError(t, kinds.Delete(ctx, graphtest.Ref(front)))

		later := func(context.Context) error {
			f.clock.Advance(time.Minute)

			return nil
		}

		kinds.On(graphtest.Disown, shared, later)
		kinds.On(graphtest.Delete, web, later)
		kinds.On(graphtest.Delete, network, later)

		root := graphtest.Ref(web)

		outcome, err := f.executor.Remove(ctx, root, 7, plan)
		require.NoError(t, err)
		require.Equal(t, execute.Done, outcome.State)

		at := func(minutes int) time.Time {
			return kindstest.Moment.Add(time.Duration(minutes) * time.Minute)
		}

		pending := func(id graph.NodeID, level int, ref graph.Ref) graph.Execution {
			return graph.Execution{
				Root:       root,
				Generation: 7,
				Node:       id,
				Kind:       id.Kind(),
				Action:     graph.ActionDelete,
				Level:      level,
				State:      graph.StatePending,
				Ref:        ref,
				UpdatedAt:  at(0),
			}
		}

		running := func(e graph.Execution, started int) graph.Execution {
			e.State = graph.StateRunning
			e.StartedAt = at(started)
			e.UpdatedAt = at(started)

			return e
		}

		done := func(e graph.Execution, finished int, reason string) graph.Execution {
			e.State = graph.StateDone
			e.Reason = reason
			e.FinishedAt = at(finished)
			e.UpdatedAt = at(finished)

			return e
		}

		ref := graphtest.Ref

		assert.Equal(t, []graph.Execution{
			pending(shared, 0, ref(shared)),
			pending(web, 1, ref(web)),
			pending(network, 2, ref(network)),
			pending(front, 2, graph.Ref{}),
			running(pending(shared, 0, ref(shared)), 0),
			done(running(pending(shared, 0, ref(shared)), 0), 1, "kept, owned by container/c_other"),
			running(pending(web, 1, ref(web)), 1),
			done(running(pending(web, 1, ref(web)), 1), 2, ""),
			running(pending(network, 2, ref(network)), 2),
			done(running(pending(network, 2, ref(network)), 2), 3, ""),
			running(pending(front, 2, graph.Ref{}), 3),
			done(running(pending(front, 2, graph.Ref{}), 3), 3, "it is gone already"),
		}, f.executions.Saved(), "every node pending first, as the record it is, and then each as it went")

		assert.Equal(t, []graph.Execution{
			done(running(pending(shared, 0, ref(shared)), 0), 1, "kept, owned by container/c_other"),
			done(running(pending(web, 1, ref(web)), 1), 2, ""),
			done(running(pending(network, 2, ref(network)), 2), 3, ""),
			done(running(pending(front, 2, graph.Ref{}), 3), 3, "it is gone already"),
		}, outcome.Nodes, "level by level, each in the order of its ids")

		kept, err := f.executions.GetByRoot(ctx, root.UUID)
		require.NoError(t, err)
		assert.Empty(t, kept, "and forgotten once the delete is done")
	})

	t.Run("the root's record goes last of all, when its kind's delete left it, and then how its plan went", func(t *testing.T) {
		t.Parallel()

		// the stack's kind leaves its record to go last.
		kinds := declared(chain)
		kinds.Leave(stack)

		f := newFixture(t, kinds)
		plan := f.plan(t, stack)
		f.made(plan)

		kinds.On(graphtest.Delete, network, func(context.Context) error {
			assert.Contains(t, kinds.Held(), stack, "the stack's record is there while what it owns goes")

			return nil
		})

		kinds.On(graphtest.Forget, stack, func(context.Context) error {
			kept, err := f.executions.GetByRoot(context.Background(), graphtest.Ref(stack).UUID)
			assert.NoError(t, err)
			assert.Len(t, kept, 5, "how its plan went is kept while its record is there")

			return nil
		})

		outcome := f.remove(t, plan)
		require.Equal(t, execute.Done, outcome.State)

		calls := kinds.Calls()
		assert.Equal(t, graphtest.Call{Op: graphtest.Forget, Node: stack}, calls[len(calls)-1], "last of all")
		assert.Equal(t, []graph.NodeID{stack}, kinds.Asked(graphtest.Forget), "once")

		assert.Equal(t, []graph.NodeID{vm, volume}, kinds.Held(), "and its record is gone")
		assert.Empty(t, f.kept(t, plan), "and then how its plan went")
	})

	t.Run("a root whose record cannot be forgotten is the error, and how its plan went is kept, for the next time", func(t *testing.T) {
		t.Parallel()

		kinds := declared(chain)
		kinds.On(graphtest.Forget, stack, func(context.Context) error { return broken })

		f := newFixture(t, kinds)
		plan := f.plan(t, stack)
		f.made(plan)

		outcome, err := f.executor.Remove(context.Background(), graphtest.Ref(stack), 1, plan)
		require.ErrorIs(t, err, broken)
		assert.EqualError(t, err, "the record of stack/s-uuid cannot be removed: broken")

		assert.Equal(t, execute.Done, outcome.State, "every node of it is done")
		assert.Len(t, f.kept(t, plan), 5)
	})

	t.Run("a plan with a node of a kind the executor is not given is removed not at all", func(t *testing.T) {
		t.Parallel()

		ctx := context.Background()
		kinds := declared(nil)

		plan, err := graph.NewPlanner(kinds.Kinds(every...), graph.DefaultPolicy()).Plan(requirement(stack))
		require.NoError(t, err)

		executions := &recorder{Repository: executionsMemory.NewRepository()}

		executor, err := execute.New(kinds.Kinds(vmKind.Name, networkKind.Name, containerKind.Name), kinds.Ownership(), executions, nil)
		require.NoError(t, err)

		_, err = executor.Remove(ctx, graphtest.Ref(stack), 1, plan)
		require.ErrorIs(t, err, kind.ErrUnknownKind)

		assert.Empty(t, executions.Saved(), "nothing of it is saved")
		assert.Empty(t, kinds.Asked(graphtest.Exists), "nor looked for")

		_, err = executor.Remove(ctx, graphtest.Ref(stack), 1, nil)
		assert.Error(t, err, "and no plan is none to carry out")
	})

	t.Run("a delete whose records, or what owns them, cannot be looked for removes nothing, and saves nothing", func(t *testing.T) {
		t.Parallel()

		for op, said := range map[graphtest.Op]string{
			graphtest.Exists: "looking for network/s_default: broken",
			graphtest.Owners: "looking for what owns network/s_default: broken",
		} {
			kinds := declared(chain)
			kinds.On(op, network, func(context.Context) error { return broken })

			f := newFixture(t, kinds)
			plan := f.plan(t, stack)
			f.made(plan)

			_, err := f.executor.Remove(context.Background(), graphtest.Ref(stack), 1, plan)
			require.ErrorIs(t, err, broken)
			assert.EqualError(t, err, said)

			assert.Empty(t, kinds.Asked(graphtest.Delete), "%s", op)
			assert.Empty(t, kinds.Asked(graphtest.Disown), "%s", op)
			assert.Empty(t, f.executions.Saved(), "%s", op)
		}
	})

	t.Run("a delete whose nodes cannot be saved pending is not started", func(t *testing.T) {
		t.Parallel()

		gone := errors.New("the database is gone")

		kinds := declared(chain)

		f := newFixture(t, kinds)
		plan := f.plan(t, stack)
		f.made(plan)

		f.executions.failWith(func(graph.Execution) error { return gone })

		_, err := f.executor.Remove(context.Background(), graphtest.Ref(stack), 1, plan)
		require.ErrorIs(t, err, gone)

		assert.Empty(t, kinds.Asked(graphtest.Delete))
		assert.Empty(t, kinds.Asked(graphtest.Forget))
	})
}

func TestExecutor_Remove_dependsOn(t *testing.T) {
	t.Parallel()

	for name, tt := range map[string]struct {
		after  map[graph.NodeID][]graph.NodeID
		levels [][]graph.NodeID
	}{
		"web after api after db goes web, then api, then db": {
			after:  chain,
			levels: [][]graph.NodeID{{stack}, {web}, {api}, {db}, {network}},
		},
		"db after api after web goes db, then api, then web": {
			after:  map[graph.NodeID][]graph.NodeID{db: {api}, api: {web}},
			levels: [][]graph.NodeID{{stack}, {db}, {api}, {web}, {network}},
		},
		"web after api and db, which need nothing of each other, goes web, and then api and db at once": {
			after:  map[graph.NodeID][]graph.NodeID{web: {api, db}},
			levels: [][]graph.NodeID{{stack}, {web}, {api, db}, {network}},
		},
		"services that depend_on nothing go at once": {
			levels: [][]graph.NodeID{{stack}, {api, db, web}, {network}},
		},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			kinds := declared(tt.after)

			// one at a time, so that a level's nodes go in the order of
			// their ids.
			f := newFixture(t, kinds, execute.Concurrency(1))
			plan := f.plan(t, stack)
			f.made(plan)

			outcome := f.remove(t, plan)
			require.Equal(t, execute.Done, outcome.State)

			assert.Equal(t, tt.levels, byLevel(outcome.Nodes))
			assert.Equal(t, slices.Concat(tt.levels...), kinds.Asked(graphtest.Delete))
		})
	}
}

func TestExecutor_Remove_policy(t *testing.T) {
	t.Parallel()

	for name, tt := range map[string]struct {
		configured string
		removed    []graph.NodeID
		left       []graph.NodeID
		unlinked   []graph.NodeID
	}{
		"by default, a stack's services and its network go, and its vm and its volume stay": {
			removed: []graph.NodeID{stack, web, api, db, network},
			left:    []graph.NodeID{vm, volume},
		},
		"with volume=owned, its volume goes too, and no kind says otherwise": {
			configured: "volume=owned",
			removed:    []graph.NodeID{stack, web, api, db, network, volume},
			left:       []graph.NodeID{vm},
		},
		"and with a stack's volumes owned alone": {
			configured: "stack/volume=owned",
			removed:    []graph.NodeID{stack, web, api, db, network, volume},
			left:       []graph.NodeID{vm},
		},
		"with network=loose, its network stays": {
			configured: "network=loose",
			removed:    []graph.NodeID{stack, web, api, db},
			left:       []graph.NodeID{network, vm, volume},
		},
		"with container=loose, its services stay, and so does the network they still own": {
			configured: "container=loose",
			removed:    []graph.NodeID{stack},
			left:       []graph.NodeID{api, db, web, network, vm, volume},
			unlinked:   []graph.NodeID{network},
		},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			kinds := declared(chain)

			f := newFixture(t, kinds)
			plan := f.planUnder(t, stack, tt.configured)
			f.made(plan)

			outcome := f.remove(t, plan)
			require.Equal(t, execute.Done, outcome.State)

			assert.ElementsMatch(t, tt.removed, kinds.Asked(graphtest.Delete))
			assert.ElementsMatch(t, tt.left, kinds.Held())

			for _, id := range tt.unlinked {
				kept, _ := outcome.Node(id)
				assert.Contains(t, kept.Reason, "kept, owned by", "%s", id)
			}
		})
	}
}

func TestExecutor_Remove_concurrency(t *testing.T) {
	t.Parallel()

	for name, tt := range map[string]struct {
		services int
		options  []execute.Option
		together int
	}{
		"a level of four is removed all at once, when four may be": {services: 4, options: []execute.Option{execute.Concurrency(4)}, together: 4},
		"never more than two at once, when two may be":             {services: 4, options: []execute.Option{execute.Concurrency(2)}, together: 2},
		"one at a time, when one may be":                           {services: 4, options: []execute.Option{execute.Concurrency(1)}, together: 1},
		"and four at a time, unless the executor is told":          {services: 6, together: 4},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			// a stack of services that need nothing: the stack, and then a
			// level of them.
			kinds := graphtest.NewKind()
			flight := newFlight(tt.together)

			var required []graph.Requirement
			for i := range tt.services {
				id := graph.ID(containerKind.Name, fmt.Sprintf("s_%d", i))
				required = append(required, requirement(id))

				kinds.On(graphtest.Delete, id, flight.call)
			}

			kinds.Require(stack, required...)

			f := newFixture(t, kinds, tt.options...)
			plan := f.plan(t, stack)
			f.made(plan)

			outcome := f.remove(t, plan)
			require.Equal(t, execute.Done, outcome.State, "%v", outcome.Err)

			assert.Equal(t, tt.together, flight.most(), "as many at once as may be, and never more")
		})
	}
}

func TestExecutor_Remove_cancelled(t *testing.T) {
	t.Parallel()

	t.Run("a delete cancelled starts nothing more, waits for what runs, and saves it pending again, as interrupted", func(t *testing.T) {
		t.Parallel()

		kinds := declared(chain)

		started := make(chan struct{})
		kinds.On(graphtest.Delete, web, func(ctx context.Context) error {
			close(started)
			<-ctx.Done()

			return ctx.Err()
		})

		f := newFixture(t, kinds)
		plan := f.plan(t, stack)
		f.made(plan)

		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		type removed struct {
			outcome execute.Outcome
			err     error
		}

		over := make(chan removed, 1)

		go func() {
			outcome, err := f.executor.Remove(ctx, graphtest.Ref(stack), 1, plan)
			over <- removed{outcome: outcome, err: err}
		}()

		select {
		case <-started:
		case <-time.After(10 * time.Second):
			require.FailNow(t, "web was never removed")
		}

		cancel()

		var result removed
		select {
		case result = <-over:
		case <-time.After(10 * time.Second):
			require.FailNow(t, "the delete never ended")
		}

		require.NoError(t, result.err)
		assert.Equal(t, execute.Interrupted, result.outcome.State)
		assert.NoError(t, result.outcome.Err, "nothing failed")

		stored := byNode(f.kept(t, plan))

		assert.Equal(t, graph.StateDone, stored[stack].State, "what was removed before stays removed")

		assert.Equal(t, graph.StatePending, stored[web].State, "what ran is pending again, and did not fail")
		assert.Equal(t, "interrupted before it was done: context canceled", stored[web].Reason)
		assert.True(t, stored[web].StartedAt.IsZero(), "as it was before it was started")
		assert.Equal(t, graphtest.Ref(web), stored[web].Ref, "the record it is")

		for _, id := range []graph.NodeID{api, db, network} {
			assert.Equal(t, graph.StatePending, stored[id].State, "%s", id)
			assert.Empty(t, stored[id].Reason, "%s is as it was", id)
		}

		assert.Equal(t, []graph.NodeID{stack, web}, kinds.Asked(graphtest.Delete), "and nothing more is started")
		assert.Empty(t, kinds.Asked(graphtest.Forget))
		assert.Contains(t, kinds.Held(), web)

		assert.Equal(t, result.outcome.Nodes, f.kept(t, plan), "the delete returned once what ran was saved")
	})

	t.Run("one cancelled before it began removes nothing", func(t *testing.T) {
		t.Parallel()

		kinds := declared(chain)

		f := newFixture(t, kinds)
		plan := f.plan(t, stack)
		f.made(plan)

		ctx, cancel := context.WithCancel(context.Background())
		cancel()

		outcome, err := f.executor.Remove(ctx, graphtest.Ref(stack), 1, plan)
		require.NoError(t, err)
		assert.Equal(t, execute.Interrupted, outcome.State)

		assert.Empty(t, kinds.Asked(graphtest.Delete))
		assert.Empty(t, kinds.Asked(graphtest.Forget))

		for _, e := range f.kept(t, plan) {
			assert.Equal(t, graph.StatePending, e.State, "%s", e.Node)
		}
	})
}

// deletes are where among the kind's events each delete began, and where it
// returned.
func deletes(kinds *graphtest.Kind) (began map[graph.NodeID]int, returned map[graph.NodeID]int) {
	began = make(map[graph.NodeID]int)
	returned = make(map[graph.NodeID]int)

	for i, event := range kinds.Events() {
		switch {
		case event.Op != graphtest.Delete:
		case event.Returned:
			returned[event.Node] = i
		default:
			began[event.Node] = i
		}
	}

	return began, returned
}
