package runTask

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/khanzadimahdi/testproject/application/runner/manager/task/schedule"
	"github.com/khanzadimahdi/testproject/domain/runner/node"
	"github.com/khanzadimahdi/testproject/domain/runner/stack"
	"github.com/khanzadimahdi/testproject/domain/runner/task"
	"github.com/khanzadimahdi/testproject/domain/runner/task/events"
	messagingMock "github.com/khanzadimahdi/testproject/infrastructure/messaging/mock"
	nodesMock "github.com/khanzadimahdi/testproject/infrastructure/repository/mocks/runner/nodes"
	stacksMock "github.com/khanzadimahdi/testproject/infrastructure/repository/mocks/runner/stacks"
	tasksMock "github.com/khanzadimahdi/testproject/infrastructure/repository/mocks/runner/tasks"
	"github.com/khanzadimahdi/testproject/infrastructure/runner/scheduler/roundrobin"
)

func created(t *testing.T, uuid string) []byte {
	t.Helper()

	payload, err := json.Marshal(events.TaskCreated{UUID: uuid})
	require.NoError(t, err)

	return payload
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
			NodeName:     "runner-worker-03",
		}

		tasks.On("GetOne", mock.Anything, service.UUID).Return(service, nil).Once()
		tasks.On("Save", mock.Anything, mock.MatchedBy(func(t *task.Task) bool {
			return t.NodeName == "runner-worker-01" && t.CurrentState == task.Scheduled
		})).Return(service.UUID, nil).Once()
		defer tasks.AssertExpectations(t)

		stacks.On("GetOne", mock.Anything, service.StackUUID).
			Return(stack.Stack{UUID: "stack-uuid", Slug: "shop-abcde", NodeName: "runner-worker-01"}, nil)
		defer stacks.AssertExpectations(t)

		var scheduled events.TaskScheduled
		producer.On("Produce", mock.Anything, events.TaskScheduledName, mock.MatchedBy(func(payload []byte) bool {
			return json.Unmarshal(payload, &scheduled) == nil
		})).Return(nil).Once()
		defer producer.AssertExpectations(t)

		handler := NewTaskCreated(&tasks, &nodes, &stacks, roundrobin.New(), schedule.New(&stacks, &producer), discardLogger())

		require.NoError(t, handler.Handle(context.Background(), created(t, service.UUID)))

		assert.Equal(t, "runner-worker-01", scheduled.NominatedNode)
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
		nodes.On("GetAll", mock.Anything, uint(0), uint(nominatedNodesLimit)).
			Return([]node.Node{{Name: "runner-worker-02", LastHeartbeatAt: time.Now()}}, nil).Once()

		// written down on the stack, so the services asked for after this one
		// find the same place.
		stacks.On("Save", mock.Anything, mock.MatchedBy(func(s *stack.Stack) bool {
			return s.NodeName == "runner-worker-02"
		})).Return("stack-uuid", nil).Once()
		defer stacks.AssertExpectations(t)

		producer.On("Produce", mock.Anything, events.TaskScheduledName, mock.Anything).Return(nil).Once()
		defer producer.AssertExpectations(t)

		handler := NewTaskCreated(&tasks, &nodes, &stacks, roundrobin.New(), schedule.New(&stacks, &producer), discardLogger())

		require.NoError(t, handler.Handle(context.Background(), created(t, service.UUID)))
	})

	t.Run("a container of its own goes where it was nominated", func(t *testing.T) {
		t.Parallel()

		var (
			tasks    tasksMock.MockTasksRepository
			nodes    nodesMock.MockNodesRepository
			stacks   stacksMock.MockStacksRepository
			producer messagingMock.MockProduceConsumer
		)

		standalone := task.Task{UUID: "task-uuid", CurrentState: task.Created, NodeName: "runner-worker-03"}

		tasks.On("GetOne", mock.Anything, standalone.UUID).Return(standalone, nil).Once()
		tasks.On("Save", mock.Anything, mock.Anything).Return(standalone.UUID, nil).Once()

		var scheduled events.TaskScheduled
		producer.On("Produce", mock.Anything, events.TaskScheduledName, mock.MatchedBy(func(payload []byte) bool {
			return json.Unmarshal(payload, &scheduled) == nil
		})).Return(nil).Once()
		defer producer.AssertExpectations(t)

		handler := NewTaskCreated(&tasks, &nodes, &stacks, roundrobin.New(), schedule.New(&stacks, &producer), discardLogger())

		require.NoError(t, handler.Handle(context.Background(), created(t, standalone.UUID)))

		assert.Equal(t, "runner-worker-03", scheduled.NominatedNode)
		stacks.AssertNotCalled(t, "GetOne", mock.Anything, mock.Anything)
	})
}
