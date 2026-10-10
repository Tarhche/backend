package execute_test

import (
	"context"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/khanzadimahdi/testproject/application/workload/controlplane/graph/execute"
	"github.com/khanzadimahdi/testproject/application/workload/controlplane/kinds/kindstest"
	"github.com/khanzadimahdi/testproject/domain/workload/graph"
	"github.com/khanzadimahdi/testproject/domain/workload/graph/graphtest"
	containerKind "github.com/khanzadimahdi/testproject/domain/workload/kinds/container"
	networkKind "github.com/khanzadimahdi/testproject/domain/workload/kinds/network"
	stackKind "github.com/khanzadimahdi/testproject/domain/workload/kinds/stack"
	vmKind "github.com/khanzadimahdi/testproject/domain/workload/kinds/vm"
	volumeKind "github.com/khanzadimahdi/testproject/domain/workload/kinds/volume"
	executionsMemory "github.com/khanzadimahdi/testproject/infrastructure/repository/memory/workload/executions"
)

// the nodes of the plans the tests apply: a container, web, in its Docker VM,
// on its network and mounting its volume; and a stack, s, of web, api and db,
// all three in one Docker VM and on the stack's network, db mounting the
// stack's volume.
var (
	vm      = graph.ID(vmKind.Name, "v-uuid")
	network = graph.ID(networkKind.Name, "s_default")
	volume  = graph.ID(volumeKind.Name, "s_data")
	web     = graph.ID(containerKind.Name, "s_web")
	api     = graph.ID(containerKind.Name, "s_api")
	db      = graph.ID(containerKind.Name, "s_db")
	stack   = graph.ID(stackKind.Name, "s-uuid")
)

// every is every kind the nodes of a plan are of.
var every = []string{vmKind.Name, networkKind.Name, volumeKind.Name, containerKind.Name, stackKind.Name}

func TestNew(t *testing.T) {
	t.Parallel()

	for name, tt := range map[string]struct {
		options []execute.Option
		refused bool
	}{
		"an executor of four nodes at a time, unless it is told otherwise": {},
		"or of as many as it is told":                                      {options: []execute.Option{execute.Concurrency(1)}},
		"but not of none":                                                  {options: []execute.Option{execute.Concurrency(0)}, refused: true},
		"nor of fewer":                                                     {options: []execute.Option{execute.Concurrency(-1)}, refused: true},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			executor, err := execute.New(graphtest.NewKind().Kinds(every...), executionsMemory.NewRepository(), nil, tt.options...)
			if tt.refused {
				assert.Error(t, err)
				assert.Nil(t, executor)

				return
			}

			require.NoError(t, err)
			assert.NotNil(t, executor)
		})
	}
}

// fixture is an executor of plans every node of which is of one fake kind,
// keeping how each node went in memory, with every save recorded, on a clock
// a test moves on.
type fixture struct {
	kinds      *graphtest.Kind
	executions *recorder
	clock      *kindstest.Clock
	executor   *execute.Executor
}

func newFixture(t *testing.T, kinds *graphtest.Kind, options ...execute.Option) *fixture {
	t.Helper()

	f := &fixture{
		kinds:      kinds,
		executions: &recorder{Repository: executionsMemory.NewRepository()},
		clock:      kindstest.NewClock(),
	}

	executor, err := execute.New(kinds.Kinds(every...), f.executions, f.clock.Now, options...)
	require.NoError(t, err)

	f.executor = executor

	return f
}

// plan is the plan of what the node id names needs, under the default
// policy.
func (f *fixture) plan(t *testing.T, id graph.NodeID) *graph.Plan {
	t.Helper()

	plan, err := graph.NewPlanner(f.kinds.Kinds(every...), graph.DefaultPolicy()).Plan(requirement(id))
	require.NoError(t, err)

	return plan
}

// apply applies plan as the first generation of its root, the record the
// fake says its root's node is, which it is no error to.
func (f *fixture) apply(t *testing.T, plan *graph.Plan) execute.Outcome {
	t.Helper()

	outcome, err := f.executor.Apply(context.Background(), graphtest.Ref(plan.Root()), 1, plan)
	require.NoError(t, err)

	return outcome
}

// kept is how each node of plan went, as it is kept.
func (f *fixture) kept(t *testing.T, plan *graph.Plan) []graph.Execution {
	t.Helper()

	kept, err := f.executions.GetByRoot(context.Background(), graphtest.Ref(plan.Root()).UUID)
	require.NoError(t, err)

	return kept
}

// recorder keeps executions in memory, and every one it is handed to save, in
// order. One fail fails is not kept, and is what it fails with.
type recorder struct {
	*executionsMemory.Repository

	lock  sync.Mutex
	saved []graph.Execution
	fail  func(graph.Execution) error
}

func (r *recorder) Save(ctx context.Context, e graph.Execution) error {
	r.lock.Lock()
	fail := r.fail
	r.lock.Unlock()

	if fail != nil {
		if err := fail(e); err != nil {
			return err
		}
	}

	r.lock.Lock()
	r.saved = append(r.saved, e)
	r.lock.Unlock()

	return r.Repository.Save(ctx, e)
}

// failWith has every save fail with what fail says, when it says anything.
func (r *recorder) failWith(fail func(graph.Execution) error) {
	r.lock.Lock()
	defer r.lock.Unlock()

	r.fail = fail
}

// Saved are the executions saved, in the order they were.
func (r *recorder) Saved() []graph.Execution {
	r.lock.Lock()
	defer r.lock.Unlock()

	return append([]graph.Execution(nil), r.saved...)
}

// requirement is the node id as a fake kind requires it, made after the
// nodes after names.
func requirement(id graph.NodeID, after ...graph.NodeID) graph.Requirement {
	r := graphtest.Requirement(id.Kind(), id.Name())
	r.After = after

	return r
}

// external is the node id as a fake kind requires it, made by somebody else.
func external(id graph.NodeID) graph.Requirement {
	r := requirement(id)
	r.External = true

	return r
}

// container is the container web, made after its vm, its network and its
// volume, which are made after the vm: its volume made by somebody else
// when externalVolume says so.
func container(externalVolume bool) *graphtest.Kind {
	mounted := requirement(volume)
	mounted.External = externalVolume

	kinds := graphtest.NewKind()
	kinds.Require(web, requirement(vm), requirement(network), mounted)
	kinds.Require(network, requirement(vm))
	kinds.Require(volume, requirement(vm))

	return kinds
}

// services is the stack s of web, api and db, each made after the services
// after says.
func services(after map[graph.NodeID][]graph.NodeID) *graphtest.Kind {
	kinds := graphtest.NewKind()
	kinds.Require(stack, requirement(web, after[web]...), requirement(api, after[api]...), requirement(db, after[db]...))
	kinds.Require(web, requirement(vm), requirement(network))
	kinds.Require(api, requirement(vm), requirement(network))
	kinds.Require(db, requirement(vm), requirement(network), requirement(volume))
	kinds.Require(network, requirement(vm))
	kinds.Require(volume, requirement(vm))

	return kinds
}

// byNode is how each node went, by node.
func byNode(executions []graph.Execution) map[graph.NodeID]graph.Execution {
	nodes := make(map[graph.NodeID]graph.Execution, len(executions))
	for _, e := range executions {
		nodes[e.Node] = e
	}

	return nodes
}

// states are the state each node went to, by node.
func states(executions []graph.Execution) map[graph.NodeID]graph.State {
	nodes := make(map[graph.NodeID]graph.State, len(executions))
	for _, e := range executions {
		nodes[e.Node] = e.State
	}

	return nodes
}
