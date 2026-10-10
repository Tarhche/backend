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

		executor, err := execute.New(kinds.Kinds(vmKind.Name, volumeKind.Name, containerKind.Name), executionsMemory.NewRepository(), nil)
		require.NoError(t, err)

		_, err = executor.Preview(context.Background(), plan)
		require.ErrorIs(t, err, kind.ErrUnknownKind)
		assert.Empty(t, kinds.Asked(graphtest.Exists))
	})
}
