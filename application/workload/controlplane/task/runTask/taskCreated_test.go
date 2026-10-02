package runTask

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/khanzadimahdi/testproject/application/workload/controlplane/task/placement"
	"github.com/khanzadimahdi/testproject/application/workload/controlplane/task/schedule"
	"github.com/khanzadimahdi/testproject/domain/workload/network"
	"github.com/khanzadimahdi/testproject/domain/workload/node"
	"github.com/khanzadimahdi/testproject/domain/workload/runtime"
	"github.com/khanzadimahdi/testproject/domain/workload/stack"
	"github.com/khanzadimahdi/testproject/domain/workload/task"
	"github.com/khanzadimahdi/testproject/domain/workload/task/events"
	messagingMock "github.com/khanzadimahdi/testproject/infrastructure/messaging/mock"
	nodesMock "github.com/khanzadimahdi/testproject/infrastructure/repository/mocks/workload/nodes"
	stacksMock "github.com/khanzadimahdi/testproject/infrastructure/repository/mocks/workload/stacks"
	tasksMock "github.com/khanzadimahdi/testproject/infrastructure/repository/mocks/workload/tasks"
	"github.com/khanzadimahdi/testproject/infrastructure/workload/scheduler/roundrobin"
)

func created(t *testing.T, uuid string) []byte {
	t.Helper()

	payload, err := json.Marshal(events.TaskCreated{UUID: uuid})
	require.NoError(t, err)

	return payload
}

func taskCreated(
	tasks *tasksMock.MockTasksRepository,
	nodes *nodesMock.MockNodesRepository,
	stacks *stacksMock.MockStacksRepository,
	producer *messagingMock.MockProduceConsumer,
) *TaskCreated {
	return NewTaskCreated(tasks, stacks, placement.New(nodes, roundrobin.New()), schedule.New(stacks, producer), producer, discardLogger())
}

func microVMs() runtime.Offer {
	return runtime.Offer{
		Class:   runtime.Firecracker,
		Driver:  "microvm",
		Healthy: true,
		Capabilities: runtime.Capabilities{
			Isolation:       runtime.IsolationMicroVM,
			NetworkPolicies: []network.Policy{network.PolicyNone, network.PolicyIsolated, network.PolicyPublic},
			StackNetworks:   true,
			ReadOnlyRoot:    true,
			TTY:             true,
			RestartPolicies: []string{"no", "always", "on-failure", "unless-stopped"},
		},
	}
}

func TestTaskCreated_Handle(t *testing.T) {
	t.Parallel()

	t.Run("a stack's services all go on the stack's node", func(t *testing.T) {
		t.Parallel()

		var (
			tasks    tasksMock.MockTasksRepository
			nodes    nodesMock.MockNodesRepository
			stacks   stacksMock.MockStacksRepository
			producer messagingMock.MockProduceConsumer
		)

		// asked for with another node in mind: the stack's is what counts,
		// because a stack's services share a network local to one node.
		service := task.Task{
			UUID:         "task-uuid",
			Name:         "shop-api",
			StackUUID:    "stack-uuid",
			ServiceName:  "api",
			CurrentState: task.Created,
			NodeName:     "workload-orchestrator-03",
		}

		tasks.On("GetOne", mock.Anything, service.UUID).Return(service, nil).Once()
		tasks.On("Save", mock.Anything, mock.MatchedBy(func(t *task.Task) bool {
			return t.NodeName == "workload-orchestrator-01" && t.CurrentState == task.Scheduled
		})).Return(service.UUID, nil).Once()
		defer tasks.AssertExpectations(t)

		stacks.On("GetOne", mock.Anything, service.StackUUID).
			Return(stack.Stack{UUID: "stack-uuid", Slug: "shop-abcde", NodeName: "workload-orchestrator-01"}, nil)
		defer stacks.AssertExpectations(t)

		var scheduled events.TaskScheduled
		producer.On("Produce", mock.Anything, events.TaskScheduledName, mock.MatchedBy(func(payload []byte) bool {
			return json.Unmarshal(payload, &scheduled) == nil
		})).Return(nil).Once()
		defer producer.AssertExpectations(t)

		require.NoError(t, taskCreated(&tasks, &nodes, &stacks, &producer).Handle(context.Background(), created(t, service.UUID)))

		assert.Equal(t, "workload-orchestrator-01", scheduled.NominatedNode)
		assert.Equal(t, "shop-abcde", scheduled.StackSlug, "and with the slug of the network they share")

		// nothing was chosen: the stack had already been placed.
		nodes.AssertNotCalled(t, "GetAll", mock.Anything, mock.Anything, mock.Anything)
		stacks.AssertNotCalled(t, "Save", mock.Anything, mock.Anything)
	})

	t.Run("the first service of an unplaced stack decides for the rest", func(t *testing.T) {
		t.Parallel()

		var (
			tasks    tasksMock.MockTasksRepository
			nodes    nodesMock.MockNodesRepository
			stacks   stacksMock.MockStacksRepository
			producer messagingMock.MockProduceConsumer
		)

		service := task.Task{UUID: "task-uuid", StackUUID: "stack-uuid", CurrentState: task.Created}

		tasks.On("GetOne", mock.Anything, service.UUID).Return(service, nil).Once()
		tasks.On("Save", mock.Anything, mock.Anything).Return(service.UUID, nil).Once()

		stacks.On("GetOne", mock.Anything, service.StackUUID).Return(stack.Stack{UUID: "stack-uuid"}, nil)
		nodes.On("GetAll", mock.Anything, uint(0), uint(placement.NodesLimit)).
			Return([]node.Node{{Name: "workload-orchestrator-02", LastHeartbeatAt: time.Now()}}, nil).Once()

		// written down on the stack, so the services asked for after this one
		// find the same place.
		stacks.On("Save", mock.Anything, mock.MatchedBy(func(s *stack.Stack) bool {
			return s.NodeName == "workload-orchestrator-02"
		})).Return("stack-uuid", nil).Once()
		defer stacks.AssertExpectations(t)

		producer.On("Produce", mock.Anything, events.TaskScheduledName, mock.Anything).Return(nil).Once()
		defer producer.AssertExpectations(t)

		require.NoError(t, taskCreated(&tasks, &nodes, &stacks, &producer).Handle(context.Background(), created(t, service.UUID)))
	})

	t.Run("a task of its own goes where it was nominated", func(t *testing.T) {
		t.Parallel()

		var (
			tasks    tasksMock.MockTasksRepository
			nodes    nodesMock.MockNodesRepository
			stacks   stacksMock.MockStacksRepository
			producer messagingMock.MockProduceConsumer
		)

		standalone := task.Task{UUID: "task-uuid", CurrentState: task.Created, NodeName: "workload-orchestrator-03"}

		tasks.On("GetOne", mock.Anything, standalone.UUID).Return(standalone, nil).Once()
		tasks.On("Save", mock.Anything, mock.Anything).Return(standalone.UUID, nil).Once()

		var scheduled events.TaskScheduled
		producer.On("Produce", mock.Anything, events.TaskScheduledName, mock.MatchedBy(func(payload []byte) bool {
			return json.Unmarshal(payload, &scheduled) == nil
		})).Return(nil).Once()
		defer producer.AssertExpectations(t)

		require.NoError(t, taskCreated(&tasks, &nodes, &stacks, &producer).Handle(context.Background(), created(t, standalone.UUID)))

		assert.Equal(t, "workload-orchestrator-03", scheduled.NominatedNode)
		stacks.AssertNotCalled(t, "GetOne", mock.Anything, mock.Anything)
	})
}

func TestTaskCreated_Handle_Runtime(t *testing.T) {
	t.Parallel()

	t.Run("a task goes to a node that offers its class, and is scheduled with it", func(t *testing.T) {
		t.Parallel()

		var (
			tasks    tasksMock.MockTasksRepository
			nodes    nodesMock.MockNodesRepository
			stacks   stacksMock.MockStacksRepository
			producer messagingMock.MockProduceConsumer
		)

		vm := task.Task{UUID: "task-uuid", Runtime: runtime.Firecracker, CurrentState: task.Created, NetworkPolicy: network.PolicyIsolated}

		tasks.On("GetOne", mock.Anything, vm.UUID).Return(vm, nil).Once()
		tasks.On("Save", mock.Anything, mock.MatchedBy(func(t *task.Task) bool {
			return t.NodeName == "workload-orchestrator-02" && t.CurrentState == task.Scheduled
		})).Return(vm.UUID, nil).Once()
		defer tasks.AssertExpectations(t)

		// the first runs sysbox alone, as every node did before there were
		// classes.
		nodes.On("GetAll", mock.Anything, uint(0), uint(placement.NodesLimit)).Return([]node.Node{
			{Name: "workload-orchestrator-01", LastHeartbeatAt: time.Now()},
			{Name: "workload-orchestrator-02", Runtimes: []runtime.Offer{microVMs()}, LastHeartbeatAt: time.Now()},
		}, nil).Once()

		var scheduled events.TaskScheduled
		producer.On("Produce", mock.Anything, events.TaskScheduledName, mock.MatchedBy(func(payload []byte) bool {
			return json.Unmarshal(payload, &scheduled) == nil
		})).Return(nil).Once()
		defer producer.AssertExpectations(t)

		require.NoError(t, taskCreated(&tasks, &nodes, &stacks, &producer).Handle(context.Background(), created(t, vm.UUID)))

		assert.Equal(t, "workload-orchestrator-02", scheduled.NominatedNode)
		assert.Equal(t, runtime.Firecracker, scheduled.Runtime)
	})

	t.Run("a task of a class no node offers fails for it, which is not an error", func(t *testing.T) {
		t.Parallel()

		var (
			tasks    tasksMock.MockTasksRepository
			nodes    nodesMock.MockNodesRepository
			stacks   stacksMock.MockStacksRepository
			producer messagingMock.MockProduceConsumer
		)

		vm := task.Task{
			UUID:         "task-uuid",
			Name:         "code-request-id",
			Runtime:      runtime.Firecracker,
			OwnerUUID:    "owner-uuid",
			MaxRetries:   3,
			CurrentState: task.Created,
		}

		tasks.On("GetOne", mock.Anything, vm.UUID).Return(vm, nil).Once()

		nodes.On("GetAll", mock.Anything, uint(0), uint(placement.NodesLimit)).Return([]node.Node{
			{Name: "workload-orchestrator-01", LastHeartbeatAt: time.Now()},
		}, nil).Once()

		// said the way a node says a task could not be run, so it is written
		// down, given up on and told to whoever is waiting, like any other.
		var failed events.TaskFailed
		producer.On("Produce", mock.Anything, events.TaskFailedName, mock.MatchedBy(func(payload []byte) bool {
			return json.Unmarshal(payload, &failed) == nil
		})).Return(nil).Once()
		defer producer.AssertExpectations(t)

		require.NoError(t, taskCreated(&tasks, &nodes, &stacks, &producer).Handle(context.Background(), created(t, vm.UUID)))

		assert.Equal(t, vm.UUID, failed.UUID)
		assert.Equal(t, vm.Name, failed.Name, "the code runner finds its snippet by its name")
		assert.Equal(t, vm.OwnerUUID, failed.OwnerUUID)
		assert.Equal(t, runtime.ReasonNoNodeOffersRuntime, failed.Reason)
		assert.True(t, failed.LastAttempt(), "no attempt is coming whatever the task was worth")

		tasks.AssertNotCalled(t, "Save", mock.Anything, mock.Anything)
	})

	t.Run("a task no node can run right now waits to be placed again", func(t *testing.T) {
		t.Parallel()

		var (
			tasks    tasksMock.MockTasksRepository
			nodes    nodesMock.MockNodesRepository
			stacks   stacksMock.MockStacksRepository
			producer messagingMock.MockProduceConsumer
		)

		vm := task.Task{UUID: "task-uuid", Runtime: runtime.Firecracker, CurrentState: task.Created}

		down := microVMs()
		down.Healthy = false

		tasks.On("GetOne", mock.Anything, vm.UUID).Return(vm, nil).Once()
		nodes.On("GetAll", mock.Anything, uint(0), uint(placement.NodesLimit)).Return([]node.Node{
			{Name: "workload-orchestrator-01", Runtimes: []runtime.Offer{down}, LastHeartbeatAt: time.Now()},
		}, nil).Once()

		require.NoError(t, taskCreated(&tasks, &nodes, &stacks, &producer).Handle(context.Background(), created(t, vm.UUID)))

		// left as it was: created, on no node, for the workload's own
		// heartbeat to ask for again.
		tasks.AssertNotCalled(t, "Save", mock.Anything, mock.Anything)
		producer.AssertNotCalled(t, "Produce", mock.Anything, mock.Anything, mock.Anything)
	})

	t.Run("so does one asked for while every node is being redeployed", func(t *testing.T) {
		t.Parallel()

		var (
			tasks    tasksMock.MockTasksRepository
			nodes    nodesMock.MockNodesRepository
			stacks   stacksMock.MockStacksRepository
			producer messagingMock.MockProduceConsumer
		)

		job := task.Task{UUID: "task-uuid", CurrentState: task.Created}

		tasks.On("GetOne", mock.Anything, job.UUID).Return(job, nil).Once()
		nodes.On("GetAll", mock.Anything, uint(0), uint(placement.NodesLimit)).Return([]node.Node{
			{Name: "workload-orchestrator-01", LastHeartbeatAt: time.Now().Add(-time.Minute)},
		}, nil).Once()

		require.NoError(t, taskCreated(&tasks, &nodes, &stacks, &producer).Handle(context.Background(), created(t, job.UUID)))

		producer.AssertNotCalled(t, "Produce", mock.Anything, mock.Anything, mock.Anything)
	})

	t.Run("a service of a stack of a class no node offers fails for it, and the stack stays unplaced", func(t *testing.T) {
		t.Parallel()

		var (
			tasks    tasksMock.MockTasksRepository
			nodes    nodesMock.MockNodesRepository
			stacks   stacksMock.MockStacksRepository
			producer messagingMock.MockProduceConsumer
		)

		service := task.Task{UUID: "task-uuid", Runtime: runtime.Firecracker, StackUUID: "stack-uuid", CurrentState: task.Created}

		tasks.On("GetOne", mock.Anything, service.UUID).Return(service, nil).Once()
		stacks.On("GetOne", mock.Anything, service.StackUUID).Return(stack.Stack{UUID: "stack-uuid", Runtime: runtime.Firecracker}, nil).Once()
		nodes.On("GetAll", mock.Anything, uint(0), uint(placement.NodesLimit)).Return([]node.Node{
			{Name: "workload-orchestrator-01", LastHeartbeatAt: time.Now()},
		}, nil).Once()
		producer.On("Produce", mock.Anything, events.TaskFailedName, mock.Anything).Return(nil).Once()
		defer producer.AssertExpectations(t)

		require.NoError(t, taskCreated(&tasks, &nodes, &stacks, &producer).Handle(context.Background(), created(t, service.UUID)))

		stacks.AssertNotCalled(t, "Save", mock.Anything, mock.Anything)
	})
}
