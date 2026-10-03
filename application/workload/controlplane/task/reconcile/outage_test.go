package reconcile

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/khanzadimahdi/testproject/application/workload/controlplane/task/schedule"
	"github.com/khanzadimahdi/testproject/domain"
	"github.com/khanzadimahdi/testproject/domain/workload/node"
	"github.com/khanzadimahdi/testproject/domain/workload/runtime"
	"github.com/khanzadimahdi/testproject/domain/workload/task"
	"github.com/khanzadimahdi/testproject/domain/workload/task/events"
	messagingMock "github.com/khanzadimahdi/testproject/infrastructure/messaging/mock"
	nodesMock "github.com/khanzadimahdi/testproject/infrastructure/repository/mocks/workload/nodes"
	stacksMock "github.com/khanzadimahdi/testproject/infrastructure/repository/mocks/workload/stacks"
	tasksMock "github.com/khanzadimahdi/testproject/infrastructure/repository/mocks/workload/tasks"
)

const grace = 2 * time.Minute

// quietFor is a VM last heard of a while ago, on a node that is still
// speaking.
func quietFor(uuid string, quiet time.Duration) task.Task {
	return task.Task{
		UUID:            uuid,
		Name:            "web",
		Runtime:         runtime.Firecracker,
		NodeName:        "workload-orchestrator-01",
		ExpectedState:   task.Running,
		CurrentState:    task.Running,
		LastHeartbeatAt: time.Now().Add(-quiet),
	}
}

// holding is the node holding them, saying its firecracker is out or not.
func holding(healthy bool, lastHeard time.Time) node.Node {
	return node.Node{
		Name: "workload-orchestrator-01",
		Runtimes: []runtime.Offer{
			{Class: runtime.Sysbox, Healthy: true},
			{Class: runtime.Firecracker, Healthy: healthy, Reason: "vmhost cannot be reached"},
		},
		LastHeartbeatAt: lastHeard,
	}
}

func TestUseCase_Execute_OutageGrace(t *testing.T) {
	t.Parallel()

	pass := func(t *testing.T, nodes *nodesMock.MockNodesRepository, producer *messagingMock.MockProduceConsumer, tasks ...task.Task) {
		t.Helper()

		var (
			repository tasksMock.MockTasksRepository
			stacks     stacksMock.MockStacksRepository
		)

		repository.On("Count", mock.Anything).Return(uint(len(tasks)), nil).Once()
		repository.On("GetAll", mock.Anything, uint(0), batch).Return(tasks, nil).Once()

		require.NoError(t, NewUseCase(&repository, nodes, schedule.New(&stacks, producer), producer, grace, discardLogger()).Execute(context.Background()))

		repository.AssertNotCalled(t, "Save", mock.Anything, mock.Anything)
	}

	t.Run("a task its node cannot see while its class is out is left as it is", func(t *testing.T) {
		t.Parallel()

		var (
			nodes    nodesMock.MockNodesRepository
			producer messagingMock.MockProduceConsumer
		)

		// vmhost is being redeployed: the VM is still running, its node
		// just cannot list it. Asking for it again would make a second one.
		nodes.On("GetOne", mock.Anything, "workload-orchestrator-01").Return(holding(false, time.Now()), nil).Once()
		defer nodes.AssertExpectations(t)

		pass(t, &nodes, &producer, quietFor("task-uuid", 30*time.Second))

		producer.AssertNotCalled(t, "Produce", mock.Anything, mock.Anything, mock.Anything)
	})

	t.Run("its node is read once, however many of its tasks have gone quiet", func(t *testing.T) {
		t.Parallel()

		var (
			nodes    nodesMock.MockNodesRepository
			producer messagingMock.MockProduceConsumer
		)

		nodes.On("GetOne", mock.Anything, "workload-orchestrator-01").Return(holding(false, time.Now()), nil).Once()
		defer nodes.AssertExpectations(t)

		pass(t, &nodes, &producer, quietFor("task-1", 30*time.Second), quietFor("task-2", 40*time.Second))

		producer.AssertNotCalled(t, "Produce", mock.Anything, mock.Anything, mock.Anything)
	})

	t.Run("nor is one asked to stop taken for stopped while its class is out", func(t *testing.T) {
		t.Parallel()

		var (
			nodes    nodesMock.MockNodesRepository
			producer messagingMock.MockProduceConsumer
		)

		nodes.On("GetOne", mock.Anything, "workload-orchestrator-01").Return(holding(false, time.Now()), nil).Once()

		stopping := quietFor("task-uuid", 30*time.Second)
		stopping.ExpectedState = task.Stopped

		pass(t, &nodes, &producer, stopping)

		producer.AssertNotCalled(t, "Produce", mock.Anything, mock.Anything, mock.Anything)
	})

	t.Run("a class out for longer than the grace is not coming back soon, and its tasks are asked for again", func(t *testing.T) {
		t.Parallel()

		var (
			nodes    nodesMock.MockNodesRepository
			producer messagingMock.MockProduceConsumer
		)

		producer.On("Produce", mock.Anything, events.TaskScheduledName, mock.Anything).Return(nil).Once()
		defer producer.AssertExpectations(t)

		pass(t, &nodes, &producer, quietFor("task-uuid", grace+time.Minute))

		nodes.AssertNotCalled(t, "GetOne", mock.Anything, mock.Anything)
	})

	t.Run("a task quiet while its class is healthy has gone, and is asked for again", func(t *testing.T) {
		t.Parallel()

		var (
			nodes    nodesMock.MockNodesRepository
			producer messagingMock.MockProduceConsumer
		)

		nodes.On("GetOne", mock.Anything, "workload-orchestrator-01").Return(holding(true, time.Now()), nil).Once()
		producer.On("Produce", mock.Anything, events.TaskScheduledName, mock.Anything).Return(nil).Once()
		defer producer.AssertExpectations(t)

		pass(t, &nodes, &producer, quietFor("task-uuid", 30*time.Second))
	})

	t.Run("a node that is not speaking either says nothing about its classes", func(t *testing.T) {
		t.Parallel()

		var (
			nodes    nodesMock.MockNodesRepository
			producer messagingMock.MockProduceConsumer
		)

		nodes.On("GetOne", mock.Anything, "workload-orchestrator-01").Return(holding(false, time.Now().Add(-time.Minute)), nil).Once()
		producer.On("Produce", mock.Anything, events.TaskScheduledName, mock.Anything).Return(nil).Once()
		defer producer.AssertExpectations(t)

		pass(t, &nodes, &producer, quietFor("task-uuid", 30*time.Second))
	})

	t.Run("a node nobody has heard of says nothing either", func(t *testing.T) {
		t.Parallel()

		var (
			nodes    nodesMock.MockNodesRepository
			producer messagingMock.MockProduceConsumer
		)

		nodes.On("GetOne", mock.Anything, "workload-orchestrator-01").Return(node.Node{}, domain.ErrNotExists).Once()
		producer.On("Produce", mock.Anything, events.TaskScheduledName, mock.Anything).Return(nil).Once()
		defer producer.AssertExpectations(t)

		pass(t, &nodes, &producer, quietFor("task-uuid", 30*time.Second))
	})

	t.Run("a task whose node cannot be read for is left for the next pass", func(t *testing.T) {
		t.Parallel()

		var (
			nodes    nodesMock.MockNodesRepository
			producer messagingMock.MockProduceConsumer
		)

		nodes.On("GetOne", mock.Anything, "workload-orchestrator-01").Return(node.Node{}, errors.New("the database is unreachable")).Once()

		pass(t, &nodes, &producer, quietFor("task-uuid", 30*time.Second))

		producer.AssertNotCalled(t, "Produce", mock.Anything, mock.Anything, mock.Anything)
	})

	t.Run("no grace takes nothing for unknown", func(t *testing.T) {
		t.Parallel()

		var (
			repository tasksMock.MockTasksRepository
			nodes      nodesMock.MockNodesRepository
			stacks     stacksMock.MockStacksRepository
			producer   messagingMock.MockProduceConsumer
		)

		quiet := quietFor("task-uuid", 30*time.Second)

		repository.On("Count", mock.Anything).Return(uint(1), nil).Once()
		repository.On("GetAll", mock.Anything, uint(0), batch).Return([]task.Task{quiet}, nil).Once()
		producer.On("Produce", mock.Anything, events.TaskScheduledName, mock.Anything).Return(nil).Once()
		defer producer.AssertExpectations(t)

		require.NoError(t, NewUseCase(&repository, &nodes, schedule.New(&stacks, &producer), &producer, 0, discardLogger()).Execute(context.Background()))

		nodes.AssertNotCalled(t, "GetOne", mock.Anything, mock.Anything)
	})
}
