package runTask

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

	deletetask "github.com/khanzadimahdi/testproject/application/runner/manager/task/deleteTask"
	"github.com/khanzadimahdi/testproject/application/runner/manager/task/schedule"
	"github.com/khanzadimahdi/testproject/domain/runner/task"
	"github.com/khanzadimahdi/testproject/domain/runner/task/events"
	messagingMock "github.com/khanzadimahdi/testproject/infrastructure/messaging/mock"
	logsMock "github.com/khanzadimahdi/testproject/infrastructure/repository/mocks/runner/logs"
	stacksMock "github.com/khanzadimahdi/testproject/infrastructure/repository/mocks/runner/stacks"
	tasksMock "github.com/khanzadimahdi/testproject/infrastructure/repository/mocks/runner/tasks"
	"github.com/khanzadimahdi/testproject/infrastructure/translator"
)

func discardLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

func failure(t *testing.T, e events.TaskFailed) []byte {
	t.Helper()

	payload, err := json.Marshal(e)
	require.NoError(t, err)

	return payload
}

func TestTaskFailed_Handle(t *testing.T) {
	t.Parallel()

	t.Run("a container that is still worth trying is asked for again", func(t *testing.T) {
		t.Parallel()

		var (
			tasks      tasksMock.MockTasksRepository
			stacks     stacksMock.MockStacksRepository
			producer   messagingMock.MockProduceConsumer
			translator translator.TranslatorMock
			logs       = logsMock.NewInMemoryRepository()
		)

		failed := task.Task{
			UUID:          "task-uuid",
			Name:          "api",
			NodeName:      "runner-worker-01",
			Kind:          task.KindService,
			MaxRetries:    3,
			CurrentState:  task.Running,
			ExpectedState: task.Running,
		}

		tasks.On("GetOne", mock.Anything, failed.UUID).Return(failed, nil).Once()

		// what it is, then that it is on its way back.
		tasks.On("Save", mock.Anything, mock.MatchedBy(func(t *task.Task) bool {
			return t.CurrentState == task.Failed &&
				t.Reason == "attempt 2 of 4: exit status 1" &&
				t.Retries == 2
		})).Return(failed.UUID, nil).Once()
		tasks.On("Save", mock.Anything, mock.MatchedBy(func(t *task.Task) bool {
			return t.CurrentState == task.Scheduled && t.ExpectedState == task.Running && t.Retries == 2
		})).Return(failed.UUID, nil).Once()
		defer tasks.AssertExpectations(t)

		var scheduled events.TaskScheduled
		producer.On("Produce", mock.Anything, events.TaskScheduledName, mock.MatchedBy(func(payload []byte) bool {
			return json.Unmarshal(payload, &scheduled) == nil
		})).Return(nil).Once()
		defer producer.AssertExpectations(t)

		handler := NewTaskFailed(
			&tasks,
			logs,
			schedule.New(&stacks, &producer),
			deletetask.NewUseCase(&tasks, logs, &producer, &translator),
			discardLogger(),
		)

		// long enough ago that the wait between attempts is over.
		require.NoError(t, handler.Handle(context.Background(), failure(t, events.TaskFailed{
			UUID:       failed.UUID,
			NodeName:   failed.NodeName,
			At:         time.Now().Add(-time.Minute),
			Attempt:    1,
			MaxRetries: failed.MaxRetries,
			Reason:     "exit status 1",
		})))

		// the next attempt, on the node that has it, counting on from the one
		// that failed.
		assert.Equal(t, 2, scheduled.Attempt)
		assert.Equal(t, failed.NodeName, scheduled.NominatedNode)

		// and the failure is where somebody looking into it will find it.
		assert.Equal(t, 1, logs.Count(failed.UUID))
	})

	t.Run("a container is left alone between attempts", func(t *testing.T) {
		t.Parallel()

		var (
			tasks      tasksMock.MockTasksRepository
			stacks     stacksMock.MockStacksRepository
			producer   messagingMock.MockProduceConsumer
			translator translator.TranslatorMock
			logs       = logsMock.NewInMemoryRepository()
		)

		failed := task.Task{
			UUID:          "task-uuid",
			Kind:          task.KindService,
			MaxRetries:    task.RetryForever,
			CurrentState:  task.Running,
			ExpectedState: task.Running,
		}

		tasks.On("GetOne", mock.Anything, failed.UUID).Return(failed, nil).Once()
		tasks.On("Save", mock.Anything, mock.Anything).Return(failed.UUID, nil).Once()
		defer tasks.AssertExpectations(t)

		handler := NewTaskFailed(
			&tasks,
			logs,
			schedule.New(&stacks, &producer),
			deletetask.NewUseCase(&tasks, logs, &producer, &translator),
			discardLogger(),
		)

		// it failed a moment ago, with attempts behind it: what it is worth is
		// another try, but not this instant.
		require.NoError(t, handler.Handle(context.Background(), failure(t, events.TaskFailed{
			UUID:       failed.UUID,
			At:         time.Now(),
			Attempt:    3,
			MaxRetries: failed.MaxRetries,
			Reason:     "exit status 1",
		})))

		producer.AssertNotCalled(t, "Produce", mock.Anything, events.TaskScheduledName, mock.Anything)

		// and the failure is written down, so the wait is measured from it
		// even if this manager is not the one that ends up making that try.
		assert.Equal(t, 1, logs.Count(failed.UUID))
	})

	t.Run("a container that has run out of attempts is left failed", func(t *testing.T) {
		t.Parallel()

		var (
			tasks      tasksMock.MockTasksRepository
			stacks     stacksMock.MockStacksRepository
			producer   messagingMock.MockProduceConsumer
			translator translator.TranslatorMock
			logs       = logsMock.NewInMemoryRepository()
		)

		failed := task.Task{
			UUID:          "task-uuid",
			Name:          "api",
			Kind:          task.KindService,
			MaxRetries:    2,
			CurrentState:  task.Running,
			ExpectedState: task.Running,
		}

		tasks.On("GetOne", mock.Anything, failed.UUID).Return(failed, nil).Once()
		tasks.On("Save", mock.Anything, mock.MatchedBy(func(t *task.Task) bool {
			return t.CurrentState == task.Failed
		})).Return(failed.UUID, nil).Once()

		// what it is, is now also what is expected of it: nothing asks again,
		// and nothing says it is going to.
		tasks.On("Save", mock.Anything, mock.MatchedBy(func(t *task.Task) bool {
			return t.ExpectedState == task.Failed && t.CurrentState == task.Failed && t.Retries == 0
		})).Return(failed.UUID, nil).Once()
		defer tasks.AssertExpectations(t)

		handler := NewTaskFailed(
			&tasks,
			logs,
			schedule.New(&stacks, &producer),
			deletetask.NewUseCase(&tasks, logs, &producer, &translator),
			discardLogger(),
		)

		require.NoError(t, handler.Handle(context.Background(), failure(t, events.TaskFailed{
			UUID:       failed.UUID,
			At:         time.Now(),
			Attempt:    2,
			MaxRetries: failed.MaxRetries,
			Reason:     "exit status 1",
		})))

		producer.AssertNotCalled(t, "Produce", mock.Anything, events.TaskScheduledName, mock.Anything)
	})

	t.Run("a job that could not run is not run again", func(t *testing.T) {
		t.Parallel()

		var (
			tasks      tasksMock.MockTasksRepository
			stacks     stacksMock.MockStacksRepository
			producer   messagingMock.MockProduceConsumer
			translator translator.TranslatorMock
			logs       = logsMock.NewInMemoryRepository()
		)

		// a job is worth one run, and what is left of it goes with it.
		job := task.Task{
			UUID:          "task-uuid",
			Name:          "code-request-id",
			Kind:          task.KindJob,
			MaxRetries:    0,
			CurrentState:  task.Scheduled,
			ExpectedState: task.Running,
		}

		// twice: once here, and once by the delete that takes it away.
		tasks.On("GetOne", mock.Anything, job.UUID).Return(job, nil).Twice()
		tasks.On("Save", mock.Anything, mock.Anything).Return(job.UUID, nil).Twice()
		tasks.On("Delete", mock.Anything, job.UUID).Return(nil).Once()
		defer tasks.AssertExpectations(t)

		producer.On("Produce", mock.Anything, events.TaskDeletedName, mock.Anything).Return(nil).Once()
		defer producer.AssertExpectations(t)

		handler := NewTaskFailed(
			&tasks,
			logs,
			schedule.New(&stacks, &producer),
			deletetask.NewUseCase(&tasks, logs, &producer, &translator),
			discardLogger(),
		)

		require.NoError(t, handler.Handle(context.Background(), failure(t, events.TaskFailed{
			UUID:       job.UUID,
			Name:       job.Name,
			At:         time.Now(),
			MaxRetries: job.MaxRetries,
			Reason:     "no nodes available",
		})))

		producer.AssertNotCalled(t, "Produce", mock.Anything, events.TaskScheduledName, mock.Anything)
	})

	t.Run("a container that failed on its way out is not brought back", func(t *testing.T) {
		t.Parallel()

		var (
			tasks      tasksMock.MockTasksRepository
			stacks     stacksMock.MockStacksRepository
			producer   messagingMock.MockProduceConsumer
			translator translator.TranslatorMock
			logs       = logsMock.NewInMemoryRepository()
		)

		// somebody asked for it to stop, and it fell over on the way.
		stopping := task.Task{
			UUID:          "task-uuid",
			Kind:          task.KindService,
			MaxRetries:    task.RetryForever,
			CurrentState:  task.Stopping,
			ExpectedState: task.Stopped,
		}

		tasks.On("GetOne", mock.Anything, stopping.UUID).Return(stopping, nil).Once()

		// only the failure is written down: what was asked of it is still what
		// was asked of it.
		tasks.On("Save", mock.Anything, mock.MatchedBy(func(t *task.Task) bool {
			return t.CurrentState == task.Failed && t.ExpectedState == task.Stopped
		})).Return(stopping.UUID, nil).Once()
		defer tasks.AssertExpectations(t)

		handler := NewTaskFailed(
			&tasks,
			logs,
			schedule.New(&stacks, &producer),
			deletetask.NewUseCase(&tasks, logs, &producer, &translator),
			discardLogger(),
		)

		require.NoError(t, handler.Handle(context.Background(), failure(t, events.TaskFailed{
			UUID:       stopping.UUID,
			At:         time.Now(),
			MaxRetries: stopping.MaxRetries,
		})))

		producer.AssertNotCalled(t, "Produce", mock.Anything, events.TaskScheduledName, mock.Anything)
	})
}
