package reconcile

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/khanzadimahdi/testproject/application/runner/manager/task/schedule"
	"github.com/khanzadimahdi/testproject/domain/runner/stack"
	"github.com/khanzadimahdi/testproject/domain/runner/task"
	"github.com/khanzadimahdi/testproject/domain/runner/task/events"
	messagingMock "github.com/khanzadimahdi/testproject/infrastructure/messaging/mock"
	stacksMock "github.com/khanzadimahdi/testproject/infrastructure/repository/mocks/runner/stacks"
	tasksMock "github.com/khanzadimahdi/testproject/infrastructure/repository/mocks/runner/tasks"
)

func discardLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

func TestUseCase_Execute(t *testing.T) {
	t.Parallel()

	t.Run("a container that is no longer where it was left is asked for again", func(t *testing.T) {
		t.Parallel()

		var (
			tasks    tasksMock.MockTasksRepository
			stacks   stacksMock.MockStacksRepository
			producer messagingMock.MockProduceConsumer
		)

		stopped := task.Task{
			UUID:            "task-uuid",
			Name:            "web",
			Image:           "nginx:1.27-alpine",
			NodeName:        "runner-worker-01",
			ExpectedState:   task.Running,
			CurrentState:    task.Stopped,
			LastHeartbeatAt: time.Now().Add(-time.Minute),
		}

		tasks.On("GetAll", mock.Anything, uint(0), limit).Return([]task.Task{stopped}, nil).Once()
		producer.On("Produce", mock.Anything, events.TaskScheduledName, mock.Anything).Return(nil).Once()
		defer producer.AssertExpectations(t)

		require.NoError(t, NewUseCase(&tasks, schedule.New(&stacks, &producer), &producer, discardLogger()).Execute(context.Background()))

		var scheduled events.TaskScheduled
		require.NoError(t, json.Unmarshal(producer.Calls[0].Arguments.Get(2).([]byte), &scheduled))

		assert.Equal(t, "task-uuid", scheduled.UUID)
		assert.Equal(t, "nginx:1.27-alpine", scheduled.Image, "it is asked for as it was asked for the first time")
		assert.Equal(t, "runner-worker-01", scheduled.NominatedNode, "on the node that was holding it")
	})

	t.Run("a container that ended while its node still holds it is left to the failure chain", func(t *testing.T) {
		t.Parallel()

		var (
			tasks    tasksMock.MockTasksRepository
			stacks   stacksMock.MockStacksRepository
			producer messagingMock.MockProduceConsumer
		)

		// it stopped without being asked to, and the node that has it said so:
		// what happens next is counted against the attempts it is worth, which
		// is not this.
		crashed := task.Task{
			UUID:            "task-uuid",
			Name:            "web",
			ExpectedState:   task.Running,
			CurrentState:    task.Stopped,
			LastHeartbeatAt: time.Now(),
		}

		tasks.On("GetAll", mock.Anything, uint(0), limit).Return([]task.Task{crashed}, nil).Once()

		require.NoError(t, NewUseCase(&tasks, schedule.New(&stacks, &producer), &producer, discardLogger()).Execute(context.Background()))

		producer.AssertNotCalled(t, "Produce", mock.Anything, mock.Anything, mock.Anything)
		tasks.AssertNotCalled(t, "Save", mock.Anything, mock.Anything)
	})

	t.Run("a container nobody has spoken for is asked for again too", func(t *testing.T) {
		t.Parallel()

		var (
			tasks    tasksMock.MockTasksRepository
			stacks   stacksMock.MockStacksRepository
			producer messagingMock.MockProduceConsumer
		)

		// last seen running, but its node has said nothing since: somebody
		// removed the container.
		vanished := task.Task{
			UUID:            "task-uuid",
			StackUUID:       "stack-uuid",
			NodeName:        "runner-worker-01",
			ExpectedState:   task.Running,
			CurrentState:    task.Running,
			LastHeartbeatAt: time.Now().Add(-time.Minute),
		}

		tasks.On("GetAll", mock.Anything, uint(0), limit).Return([]task.Task{vanished}, nil).Once()
		stacks.On("GetOne", mock.Anything, "stack-uuid").Return(stack.Stack{UUID: "stack-uuid", Slug: "myapp-abcde"}, nil).Once()
		producer.On("Produce", mock.Anything, events.TaskScheduledName, mock.Anything).Return(nil).Once()
		defer producer.AssertExpectations(t)

		require.NoError(t, NewUseCase(&tasks, schedule.New(&stacks, &producer), &producer, discardLogger()).Execute(context.Background()))

		var scheduled events.TaskScheduled
		require.NoError(t, json.Unmarshal(producer.Calls[0].Arguments.Get(2).([]byte), &scheduled))

		assert.Equal(t, "myapp-abcde", scheduled.StackSlug, "a service comes back onto its stack's own network")
	})

	t.Run("a container that came back up after it was stopped is stopped again", func(t *testing.T) {
		t.Parallel()

		var (
			tasks    tasksMock.MockTasksRepository
			stacks   stacksMock.MockStacksRepository
			producer messagingMock.MockProduceConsumer
		)

		revived := task.Task{
			UUID:            "task-uuid",
			ExpectedState:   task.Stopped,
			CurrentState:    task.Running,
			LastHeartbeatAt: time.Now(),
		}

		tasks.On("GetAll", mock.Anything, uint(0), limit).Return([]task.Task{revived}, nil).Once()
		producer.On("Produce", mock.Anything, events.TaskStoppageRequestedName, mock.Anything).Return(nil).Once()
		defer producer.AssertExpectations(t)

		require.NoError(t, NewUseCase(&tasks, schedule.New(&stacks, &producer), &producer, discardLogger()).Execute(context.Background()))
	})

	t.Run("containers that are what they were asked to be are left alone", func(t *testing.T) {
		t.Parallel()

		var (
			tasks    tasksMock.MockTasksRepository
			stacks   stacksMock.MockStacksRepository
			producer messagingMock.MockProduceConsumer
		)

		now := time.Now()

		tasks.On("GetAll", mock.Anything, uint(0), limit).Return([]task.Task{
			{UUID: "running", ExpectedState: task.Running, CurrentState: task.Running, LastHeartbeatAt: now},
			{UUID: "stopping", ExpectedState: task.Stopped, CurrentState: task.Stopping, LastHeartbeatAt: now},
			{UUID: "a finished job", Kind: task.KindJob, ExpectedState: task.Completed, CurrentState: task.Completed, LastHeartbeatAt: now},
		}, nil).Once()

		require.NoError(t, NewUseCase(&tasks, schedule.New(&stacks, &producer), &producer, discardLogger()).Execute(context.Background()))

		producer.AssertNotCalled(t, "Produce", mock.Anything, mock.Anything, mock.Anything)
	})
}

func TestUseCase_Execute_settling(t *testing.T) {
	t.Parallel()

	t.Run("a container asked to stop, that nobody holds any more, has stopped", func(t *testing.T) {
		t.Parallel()

		var (
			tasks    tasksMock.MockTasksRepository
			stacks   stacksMock.MockStacksRepository
			producer messagingMock.MockProduceConsumer
		)

		// it was asked to stop, and then its node stopped speaking for it:
		// there is nothing left to stop.
		vanished := task.Task{
			UUID:            "task-uuid",
			ExpectedState:   task.Stopped,
			CurrentState:    task.Stopping,
			LastHeartbeatAt: time.Now().Add(-time.Minute),
		}

		tasks.On("GetAll", mock.Anything, uint(0), limit).Return([]task.Task{vanished}, nil).Once()
		tasks.On("Save", mock.Anything, mock.MatchedBy(func(t *task.Task) bool {
			return t.CurrentState == task.Stopped
		})).Return("task-uuid", nil).Once()
		defer tasks.AssertExpectations(t)

		require.NoError(t, NewUseCase(&tasks, schedule.New(&stacks, &producer), &producer, discardLogger()).Execute(context.Background()))

		producer.AssertNotCalled(t, "Produce", mock.Anything, mock.Anything, mock.Anything)
	})

	t.Run("a job that finished is not asked to stop again", func(t *testing.T) {
		t.Parallel()

		var (
			tasks    tasksMock.MockTasksRepository
			stacks   stacksMock.MockStacksRepository
			producer messagingMock.MockProduceConsumer
		)

		// somebody stopped it as it was finishing: finished is not running,
		// which is what stopping asked for.
		finished := task.Task{
			UUID:            "task-uuid",
			Kind:            task.KindJob,
			ExpectedState:   task.Stopped,
			CurrentState:    task.Completed,
			LastHeartbeatAt: time.Now(),
		}

		tasks.On("GetAll", mock.Anything, uint(0), limit).Return([]task.Task{finished}, nil).Once()

		require.NoError(t, NewUseCase(&tasks, schedule.New(&stacks, &producer), &producer, discardLogger()).Execute(context.Background()))

		producer.AssertNotCalled(t, "Produce", mock.Anything, mock.Anything, mock.Anything)
		tasks.AssertNotCalled(t, "Save", mock.Anything, mock.Anything)
	})
}

func TestUseCase_Execute_placing(t *testing.T) {
	t.Parallel()

	t.Run("a container that was never placed anywhere is asked for from the beginning", func(t *testing.T) {
		t.Parallel()

		var (
			tasks    tasksMock.MockTasksRepository
			stacks   stacksMock.MockStacksRepository
			producer messagingMock.MockProduceConsumer
		)

		// asked for when there was nowhere to put it: no node was chosen, so
		// there is nobody to ask for it again.
		unplaced := task.Task{
			UUID:          "task-uuid",
			Name:          "web",
			ExpectedState: task.Running,
			CurrentState:  task.Created,
			CreatedAt:     time.Now().Add(-time.Minute),
		}

		tasks.On("GetAll", mock.Anything, uint(0), limit).Return([]task.Task{unplaced}, nil).Once()
		producer.On("Produce", mock.Anything, events.TaskCreatedName, mock.Anything).Return(nil).Once()
		defer producer.AssertExpectations(t)

		require.NoError(t, NewUseCase(&tasks, schedule.New(&stacks, &producer), &producer, discardLogger()).Execute(context.Background()))

		producer.AssertNotCalled(t, "Produce", mock.Anything, events.TaskScheduledName, mock.Anything)
	})
}
