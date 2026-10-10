package execute_test

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/khanzadimahdi/testproject/application/workload/controlplane/graph/execute"
	"github.com/khanzadimahdi/testproject/domain/workload/graph"
	"github.com/khanzadimahdi/testproject/domain/workload/graph/graphtest"
	"github.com/khanzadimahdi/testproject/domain/workload/kind"
	containerKind "github.com/khanzadimahdi/testproject/domain/workload/kinds/container"
	networkKind "github.com/khanzadimahdi/testproject/domain/workload/kinds/network"
	stackKind "github.com/khanzadimahdi/testproject/domain/workload/kinds/stack"
	vmKind "github.com/khanzadimahdi/testproject/domain/workload/kinds/vm"
	volumeKind "github.com/khanzadimahdi/testproject/domain/workload/kinds/volume"
	executionsMemory "github.com/khanzadimahdi/testproject/infrastructure/repository/memory/workload/executions"
)

func TestExecutor_Preview(t *testing.T) {
	t.Parallel()

	t.Run("what is there, what would be made, and which external node would be waited on, read and not done", func(t *testing.T) {
		t.Parallel()

		front := graph.ID(networkKind.Name, "s_front")

		// web needs its vm, which is there; its network, which is not; an
		// external network, which is; and an external volume, which is not.
		kinds := graphtest.NewKind()
		kinds.Require(web, requirement(vm), requirement(network), external(front), external(volume))
		kinds.Require(network, requirement(vm))
		kinds.Require(front, requirement(vm))
		kinds.Require(volume, requirement(vm))
		kinds.Hold(vm, front)

		f := newFixture(t, kinds)

		preview, err := f.executor.Preview(context.Background(), f.plan(t, web))
		require.NoError(t, err)

		assert.Equal(t, execute.Preview{Levels: [][]execute.Previewed{
			{
				{Node: vm, Kind: vmKind.Name, Verdict: execute.Exists, Ref: graphtest.Ref(vm)},
			},
			{
				{Node: network, Kind: networkKind.Name, Verdict: execute.Create},
				{Node: front, Kind: networkKind.Name, Verdict: execute.Exists, External: true, Ref: graphtest.Ref(front)},
				{Node: volume, Kind: volumeKind.Name, Verdict: execute.Wait, External: true},
			},
			{
				{Node: web, Kind: containerKind.Name, Verdict: execute.Create},
			},
		}}, preview)

		assert.ElementsMatch(t, []graph.NodeID{vm, network, front, volume, web}, kinds.Asked(graphtest.Exists), "every node is looked for")
		assert.Empty(t, kinds.Asked(graphtest.Create), "and none is made")
		assert.Empty(t, f.executions.Saved(), "nor saved")

		served, err := json.Marshal(preview)
		require.NoError(t, err)

		assert.JSONEq(t, `{"levels": [
			[
				{"node": "vm/v-uuid", "kind": "vm", "verdict": "exists", "ref": {"kind": "vm", "uuid": "v-uuid"}}
			],
			[
				{"node": "network/s_default", "kind": "network", "verdict": "create"},
				{"node": "network/s_front", "kind": "network", "verdict": "exists", "external": true, "ref": {"kind": "network", "uuid": "s_front"}},
				{"node": "volume/s_data", "kind": "volume", "verdict": "wait", "external": true}
			],
			[
				{"node": "container/s_web", "kind": "container", "verdict": "create"}
			]
		]}`, string(served), "as it is served")
	})

	t.Run("a node whose record is there and not made would be made, and is said with its record", func(t *testing.T) {
		t.Parallel()

		// web's network is being made, and its external volume is going
		// away.
		kinds := container(true)
		kinds.Hold(vm)
		kinds.Record(network, volume)

		f := newFixture(t, kinds)

		preview, err := f.executor.Preview(context.Background(), f.plan(t, web))
		require.NoError(t, err)

		assert.Equal(t, execute.Preview{Levels: [][]execute.Previewed{
			{
				{Node: vm, Kind: vmKind.Name, Verdict: execute.Exists, Ref: graphtest.Ref(vm)},
			},
			{
				{Node: network, Kind: networkKind.Name, Verdict: execute.Create, Ref: graphtest.Ref(network)},
				{Node: volume, Kind: volumeKind.Name, Verdict: execute.Wait, External: true, Ref: graphtest.Ref(volume)},
			},
			{
				{Node: web, Kind: containerKind.Name, Verdict: execute.Create},
			},
		}}, preview)
	})

	t.Run("a plan made already would make nothing", func(t *testing.T) {
		t.Parallel()

		f := newFixture(t, container(false))
		plan := f.plan(t, web)

		require.Equal(t, execute.Done, f.apply(t, plan).State)

		made := f.kinds.Asked(graphtest.Create)
		saved := f.executions.Saved()

		preview, err := f.executor.Preview(context.Background(), plan)
		require.NoError(t, err)

		require.Len(t, preview.Levels, 3)
		for _, level := range preview.Levels {
			for _, previewed := range level {
				assert.Equal(t, execute.Exists, previewed.Verdict, "%s", previewed.Node)
				assert.Equal(t, graphtest.Ref(previewed.Node), previewed.Ref)
			}
		}

		assert.Equal(t, made, f.kinds.Asked(graphtest.Create))
		assert.Equal(t, saved, f.executions.Saved())
	})

	t.Run("what looking for a node fails with is an error naming it", func(t *testing.T) {
		t.Parallel()

		broken := errors.New("broken")

		kinds := container(false)
		kinds.On(graphtest.Exists, network, func(context.Context) error { return broken })

		f := newFixture(t, kinds)

		_, err := f.executor.Preview(context.Background(), f.plan(t, web))
		require.ErrorIs(t, err, broken)
		assert.EqualError(t, err, "whether network/s_default is there: broken")
	})

	t.Run("a plan with a node of a kind the executor is not given is not previewed", func(t *testing.T) {
		t.Parallel()

		kinds := container(false)

		plan, err := graph.NewPlanner(kinds.Kinds(every...), graph.DefaultPolicy()).Plan(requirement(web))
		require.NoError(t, err)

		executor, err := execute.New(kinds.Kinds(vmKind.Name, volumeKind.Name, containerKind.Name), kinds.Ownership(), executionsMemory.NewRepository(), nil)
		require.NoError(t, err)

		_, err = executor.Preview(context.Background(), plan)
		require.ErrorIs(t, err, kind.ErrUnknownKind)
		assert.Empty(t, kinds.Asked(graphtest.Exists))
	})
}

func TestExecutor_PreviewRemove(t *testing.T) {
	t.Parallel()

	t.Run("what removing would remove, unlink and never touch, and what is gone already, read and not done", func(t *testing.T) {
		t.Parallel()

		ctx := context.Background()

		// the stack s, web made after api and api after db; web on a
		// network another root shares too, and on one of somebody else's;
		// and db gone already.
		kinds := declared(chain)
		kinds.Require(web, requirement(vm), requirement(network), requirement(shared), external(front))
		kinds.Require(other, requirement(vm), requirement(shared))
		kinds.Require(shared, requirement(vm))
		kinds.Require(front, requirement(vm))

		f := newFixture(t, kinds)
		plan := f.plan(t, stack)
		f.made(plan, f.plan(t, other))

		require.NoError(t, kinds.Delete(ctx, graphtest.Ref(db)))

		asked := len(kinds.Calls())
		held := kinds.Held()

		preview, err := f.executor.PreviewRemove(ctx, plan)
		require.NoError(t, err)

		ref := graphtest.Ref

		assert.Equal(t, execute.Preview{Levels: [][]execute.Previewed{
			{
				{Node: shared, Kind: networkKind.Name, Verdict: execute.Unlink, Ref: ref(shared)},
			},
			{
				{Node: stack, Kind: stackKind.Name, Verdict: execute.Delete, Ref: ref(stack)},
			},
			{
				{Node: web, Kind: containerKind.Name, Verdict: execute.Delete, Ref: ref(web)},
			},
			{
				{Node: api, Kind: containerKind.Name, Verdict: execute.Delete, Ref: ref(api)},
			},
			{
				{Node: db, Kind: containerKind.Name, Verdict: execute.Gone},
			},
			{
				{Node: network, Kind: networkKind.Name, Verdict: execute.Delete, Ref: ref(network)},
			},
			{
				{Node: front, Kind: networkKind.Name, Verdict: execute.Keep, External: true},
				{Node: vm, Kind: vmKind.Name, Verdict: execute.Keep},
				{Node: volume, Kind: volumeKind.Name, Verdict: execute.Keep},
			},
		}}, preview, "unlinked first, then removed from the top down, and last what is never touched")

		for _, call := range kinds.Calls()[asked:] {
			assert.Contains(t, []graphtest.Op{graphtest.Exists, graphtest.Owners}, call.Op, "only looked for, and for what owns it: %s of %s", call.Op, call.Node)
			assert.NotContains(t, []graph.NodeID{vm, volume, front}, call.Node, "and what it never touches is not even that")
		}

		assert.Equal(t, held, kinds.Held(), "nothing is removed")
		assert.Equal(t, []graph.NodeID{other, web}, f.ownership.OwnersOf(shared), "nor unlinked")
		assert.Empty(t, f.executions.Saved(), "nor saved")

		served, err := json.Marshal(preview)
		require.NoError(t, err)

		assert.JSONEq(t, `{"levels": [
			[
				{"node": "network/shared", "kind": "network", "verdict": "unlink", "ref": {"kind": "network", "uuid": "shared"}}
			],
			[
				{"node": "stack/s-uuid", "kind": "stack", "verdict": "delete", "ref": {"kind": "stack", "uuid": "s-uuid"}}
			],
			[
				{"node": "container/s_web", "kind": "container", "verdict": "delete", "ref": {"kind": "container", "uuid": "s_web"}}
			],
			[
				{"node": "container/s_api", "kind": "container", "verdict": "delete", "ref": {"kind": "container", "uuid": "s_api"}}
			],
			[
				{"node": "container/s_db", "kind": "container", "verdict": "gone"}
			],
			[
				{"node": "network/s_default", "kind": "network", "verdict": "delete", "ref": {"kind": "network", "uuid": "s_default"}}
			],
			[
				{"node": "network/s_front", "kind": "network", "verdict": "keep", "external": true},
				{"node": "vm/v-uuid", "kind": "vm", "verdict": "keep"},
				{"node": "volume/s_data", "kind": "volume", "verdict": "keep"}
			]
		]}`, string(served), "as it is served")
	})

	t.Run("and a delete would remove what the policy says the root owns, and no kind does", func(t *testing.T) {
		t.Parallel()

		kinds := declared(chain)

		f := newFixture(t, kinds)
		plan := f.planUnder(t, stack, "volume=owned")
		f.made(plan)

		preview, err := f.executor.PreviewRemove(context.Background(), plan)
		require.NoError(t, err)

		verdicts := make(map[graph.NodeID]execute.Verdict)
		for _, level := range preview.Levels {
			for _, previewed := range level {
				verdicts[previewed.Node] = previewed.Verdict
			}
		}

		assert.Equal(t, map[graph.NodeID]execute.Verdict{
			stack:   execute.Delete,
			web:     execute.Delete,
			api:     execute.Delete,
			db:      execute.Delete,
			network: execute.Delete,
			volume:  execute.Delete,
			vm:      execute.Keep,
		}, verdicts)
	})

	t.Run("what looking for a node, or for what owns it, fails with is an error naming it", func(t *testing.T) {
		t.Parallel()

		broken := errors.New("broken")

		for op, said := range map[graphtest.Op]string{
			graphtest.Exists: "looking for network/s_default: broken",
			graphtest.Owners: "looking for what owns network/s_default: broken",
		} {
			kinds := declared(chain)
			kinds.On(op, network, func(context.Context) error { return broken })

			f := newFixture(t, kinds)
			plan := f.plan(t, stack)
			f.made(plan)

			_, err := f.executor.PreviewRemove(context.Background(), plan)
			require.ErrorIs(t, err, broken)
			assert.EqualError(t, err, said)
		}
	})

	t.Run("a plan with a node of a kind the executor is not given is not previewed", func(t *testing.T) {
		t.Parallel()

		kinds := declared(nil)

		plan, err := graph.NewPlanner(kinds.Kinds(every...), graph.DefaultPolicy()).Plan(requirement(stack))
		require.NoError(t, err)

		executor, err := execute.New(kinds.Kinds(vmKind.Name, volumeKind.Name, containerKind.Name, stackKind.Name), kinds.Ownership(), executionsMemory.NewRepository(), nil)
		require.NoError(t, err)

		_, err = executor.PreviewRemove(context.Background(), plan)
		require.ErrorIs(t, err, kind.ErrUnknownKind)
		assert.Empty(t, kinds.Asked(graphtest.Exists))
	})
}
