package heartbeatTask

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	deletetask "github.com/khanzadimahdi/testproject/application/workload/controlplane/task/deleteTask"
	killtask "github.com/khanzadimahdi/testproject/application/workload/controlplane/task/killTask"
	"github.com/khanzadimahdi/testproject/application/workload/controlplane/task/runTask"
	"github.com/khanzadimahdi/testproject/domain"
	"github.com/khanzadimahdi/testproject/domain/workload/task"
	"github.com/khanzadimahdi/testproject/domain/workload/task/events"
	messagingMock "github.com/khanzadimahdi/testproject/infrastructure/messaging/mock"
	tasksMemory "github.com/khanzadimahdi/testproject/infrastructure/repository/memory/workload/tasks"
	logsMock "github.com/khanzadimahdi/testproject/infrastructure/repository/mocks/workload/logs"
	tasksMock "github.com/khanzadimahdi/testproject/infrastructure/repository/mocks/workload/tasks"
	"github.com/khanzadimahdi/testproject/infrastructure/translator"
)

const taskUUID = "task-uuid"

func beat(t *testing.T, state task.State) []byte {
	t.Helper()

	return beatAt(t, state, time.Now())
}

func beatAt(t *testing.T, state task.State, at time.Time) []byte {
	t.Helper()

	payload, err := json.Marshal(events.Heartbeat{
		UUID:     taskUUID,
		Name:     "a-name",
		Kind:     string(task.KindJob),
		State:    int(state),
		NodeName: "workload-orchestrator-01",
		At:       at,
	})
	require.NoError(t, err)

	return payload
}

// handler builds the heartbeat handler with the pieces it leans on.
func handler(tasks task.Repository, producer domain.Producer, logs task.LogRepository) *Heartbeat {
	words := &translator.TranslatorMock{}

	return NewHeartbeatHandler(
		tasks,
		producer,
		deletetask.NewUseCase(tasks, logs, producer, words),
		killtask.NewUseCase(tasks, producer, words),
	)
}

func TestHeartbeat_Handle(t *testing.T) {
	t.Parallel()

	t.Run("a task that was only meant to run once is taken away when it finishes", func(t *testing.T) {
		t.Parallel()

		var (
			tasks    tasksMock.MockTasksRepository
			producer messagingMock.MockProduceConsumer
		)

		logs := logsMock.NewInMemoryRepository()
		finished := task.Task{UUID: taskUUID, CurrentState: task.Running, AutoRemove: true}

		tasks.On("GetOne", mock.Anything, taskUUID).Return(finished, nil)
		tasks.On("Save", mock.Anything, mock.Anything).Return(taskUUID, nil)
		tasks.On("Delete", mock.Anything, taskUUID).Return(nil).Once()
		producer.On("Produce", mock.Anything, mock.Anything, mock.Anything).Return(nil)
		defer tasks.AssertExpectations(t)

		require.NoError(t, handler(&tasks, &producer, logs).Handle(context.Background(), beat(t, task.Completed)))

		producer.AssertCalled(t, "Produce", mock.Anything, events.TaskCompletedName, mock.Anything)
		producer.AssertCalled(t, "Produce", mock.Anything, events.TaskDeletedName, mock.Anything)
	})

	t.Run("a task meant to keep running is left alone when it stops", func(t *testing.T) {
		t.Parallel()

		var (
			tasks    tasksMock.MockTasksRepository
			producer messagingMock.MockProduceConsumer
		)

		logs := logsMock.NewInMemoryRepository()
		stopped := task.Task{UUID: taskUUID, CurrentState: task.Running, AutoRemove: false}

		tasks.On("GetOne", mock.Anything, taskUUID).Return(stopped, nil)
		tasks.On("Save", mock.Anything, mock.Anything).Return(taskUUID, nil)
		producer.On("Produce", mock.Anything, mock.Anything, mock.Anything).Return(nil)

		require.NoError(t, handler(&tasks, &producer, logs).Handle(context.Background(), beat(t, task.Stopped)))

		// a stopped service is still a task somebody can start again.
		tasks.AssertNotCalled(t, "Delete", mock.Anything, mock.Anything)
		producer.AssertNotCalled(t, "Produce", mock.Anything, events.TaskDeletedName, mock.Anything)
		producer.AssertCalled(t, "Produce", mock.Anything, events.TaskStoppedName, mock.Anything)
	})

	t.Run("a job that has run for longer than it asked for is stopped", func(t *testing.T) {
		t.Parallel()

		var (
			tasks    tasksMock.MockTasksRepository
			producer messagingMock.MockProduceConsumer
		)

		started := time.Now().Add(-time.Minute)
		overrunning := task.Task{
			UUID:         taskUUID,
			Kind:         task.KindJob,
			CurrentState: task.Running,
			StartedAt:    started,
			TTL:          30 * time.Second,
		}

		tasks.On("GetOne", mock.Anything, taskUUID).Return(overrunning, nil)
		tasks.On("Save", mock.Anything, mock.Anything).Return(taskUUID, nil)
		producer.On("Produce", mock.Anything, mock.Anything, mock.Anything).Return(nil)

		require.NoError(t, handler(&tasks, &producer, logsMock.NewInMemoryRepository()).Handle(context.Background(), beatAt(t, task.Running, time.Now())))

		producer.AssertCalled(t, "Produce", mock.Anything, events.TaskKillRequestedName, mock.Anything)
	})

	t.Run("a job still inside the time it asked for is left to run", func(t *testing.T) {
		t.Parallel()

		var (
			tasks    tasksMock.MockTasksRepository
			producer messagingMock.MockProduceConsumer
		)

		running := task.Task{
			UUID:         taskUUID,
			Kind:         task.KindJob,
			CurrentState: task.Running,
			StartedAt:    time.Now().Add(-time.Second),
			TTL:          30 * time.Second,
		}

		tasks.On("GetOne", mock.Anything, taskUUID).Return(running, nil)
		tasks.On("Save", mock.Anything, mock.Anything).Return(taskUUID, nil)
		producer.On("Produce", mock.Anything, mock.Anything, mock.Anything).Return(nil)

		require.NoError(t, handler(&tasks, &producer, logsMock.NewInMemoryRepository()).Handle(context.Background(), beatAt(t, task.Running, time.Now())))

		producer.AssertNotCalled(t, "Produce", mock.Anything, events.TaskKillRequestedName, mock.Anything)
	})

	t.Run("a service is never stopped for running too long", func(t *testing.T) {
		t.Parallel()

		var (
			tasks    tasksMock.MockTasksRepository
			producer messagingMock.MockProduceConsumer
		)

		// a ttl means nothing to something meant to keep running, even if one
		// somehow found its way onto it.
		service := task.Task{
			UUID:         taskUUID,
			Kind:         task.KindService,
			CurrentState: task.Running,
			StartedAt:    time.Now().Add(-time.Hour),
			TTL:          time.Second,
		}

		tasks.On("GetOne", mock.Anything, taskUUID).Return(service, nil)
		tasks.On("Save", mock.Anything, mock.Anything).Return(taskUUID, nil)
		producer.On("Produce", mock.Anything, mock.Anything, mock.Anything).Return(nil)

		require.NoError(t, handler(&tasks, &producer, logsMock.NewInMemoryRepository()).Handle(context.Background(), beatAt(t, task.Running, time.Now())))

		producer.AssertNotCalled(t, "Produce", mock.Anything, events.TaskKillRequestedName, mock.Anything)
	})
}

func TestHeartbeat_Handle_failing(t *testing.T) {
	t.Parallel()

	beatOfAttempt := func(t *testing.T, state task.State, attempt int) []byte {
		t.Helper()

		payload, err := json.Marshal(events.Heartbeat{
			UUID:     taskUUID,
			Name:     "a-name",
			Kind:     string(task.KindService),
			State:    int(state),
			NodeName: "workload-orchestrator-01",
			Attempt:  attempt,
			At:       time.Now(),
		})
		require.NoError(t, err)

		return payload
	}

	t.Run("a service that ended without being asked to has failed to stay up", func(t *testing.T) {
		t.Parallel()

		var (
			tasks    tasksMock.MockTasksRepository
			producer messagingMock.MockProduceConsumer
		)

		crashed := task.Task{
			UUID:          taskUUID,
			Kind:          task.KindService,
			MaxRetries:    3,
			CurrentState:  task.Running,
			ExpectedState: task.Running,
		}

		tasks.On("GetOne", mock.Anything, taskUUID).Return(crashed, nil)
		tasks.On("Save", mock.Anything, mock.Anything).Return(taskUUID, nil)

		var failed events.TaskFailed
		producer.On("Produce", mock.Anything, events.TaskFailedName, mock.MatchedBy(func(payload []byte) bool {
			return json.Unmarshal(payload, &failed) == nil
		})).Return(nil).Once()
		defer producer.AssertExpectations(t)

		require.NoError(t, handler(&tasks, &producer, logsMock.NewInMemoryRepository()).
			Handle(context.Background(), beatOfAttempt(t, task.Stopped, 1)))

		// with the attempts behind it, so that whoever decides on another one
		// knows how many there have been.
		require.Equal(t, 1, failed.Attempt)
		require.Equal(t, 3, failed.MaxRetries)

		producer.AssertNotCalled(t, "Produce", mock.Anything, events.TaskStoppedName, mock.Anything)
	})

	t.Run("a service that was asked to stop has stopped", func(t *testing.T) {
		t.Parallel()

		var (
			tasks    tasksMock.MockTasksRepository
			producer messagingMock.MockProduceConsumer
		)

		stopping := task.Task{
			UUID:          taskUUID,
			Kind:          task.KindService,
			MaxRetries:    3,
			CurrentState:  task.Stopping,
			ExpectedState: task.Stopped,
		}

		tasks.On("GetOne", mock.Anything, taskUUID).Return(stopping, nil)
		tasks.On("Save", mock.Anything, mock.Anything).Return(taskUUID, nil)
		producer.On("Produce", mock.Anything, events.TaskStoppedName, mock.Anything).Return(nil).Once()
		defer producer.AssertExpectations(t)

		require.NoError(t, handler(&tasks, &producer, logsMock.NewInMemoryRepository()).
			Handle(context.Background(), beatOfAttempt(t, task.Stopped, 0)))

		producer.AssertNotCalled(t, "Produce", mock.Anything, events.TaskFailedName, mock.Anything)
	})

	t.Run("a task that has already failed is not answered again", func(t *testing.T) {
		t.Parallel()

		var (
			tasks    tasksMock.MockTasksRepository
			producer messagingMock.MockProduceConsumer
		)

		// it says so in every report it makes until somebody takes it away.
		failed := task.Task{
			UUID:          taskUUID,
			Kind:          task.KindService,
			CurrentState:  task.Failed,
			ExpectedState: task.Running,
		}

		tasks.On("GetOne", mock.Anything, taskUUID).Return(failed, nil)
		tasks.On("Save", mock.Anything, mock.Anything).Return(taskUUID, nil)

		require.NoError(t, handler(&tasks, &producer, logsMock.NewInMemoryRepository()).
			Handle(context.Background(), beatOfAttempt(t, task.Failed, 0)))

		producer.AssertNotCalled(t, "Produce", mock.Anything, mock.Anything, mock.Anything)
	})
}

func TestHeartbeat_Handle_askingAgain(t *testing.T) {
	t.Parallel()

	beatOfFailure := func(t *testing.T, attempt int) []byte {
		t.Helper()

		payload, err := json.Marshal(events.Heartbeat{
			UUID:     taskUUID,
			Kind:     string(task.KindService),
			State:    int(task.Failed),
			NodeName: "workload-orchestrator-01",
			Attempt:  attempt,
			At:       time.Now(),
		})
		require.NoError(t, err)

		return payload
	}

	// a task that has failed goes on saying so, and the node holding it
	// goes on reporting it: that is what asks for the next attempt once the
	// wait between attempts is over.
	failed := func(finishedAt time.Time) task.Task {
		return task.Task{
			UUID:          taskUUID,
			Kind:          task.KindService,
			MaxRetries:    task.RetryForever,
			CurrentState:  task.Failed,
			ExpectedState: task.Running,
			FinishedAt:    finishedAt,
		}
	}

	t.Run("a task left long enough is asked about again", func(t *testing.T) {
		t.Parallel()

		var (
			tasks    tasksMock.MockTasksRepository
			producer messagingMock.MockProduceConsumer
		)

		tasks.On("GetOne", mock.Anything, taskUUID).Return(failed(time.Now().Add(-time.Minute)), nil)
		tasks.On("Save", mock.Anything, mock.Anything).Return(taskUUID, nil)
		producer.On("Produce", mock.Anything, events.TaskFailedName, mock.Anything).Return(nil).Once()
		defer producer.AssertExpectations(t)

		require.NoError(t, handler(&tasks, &producer, logsMock.NewInMemoryRepository()).
			Handle(context.Background(), beatOfFailure(t, 2)))
	})

	t.Run("a task that has just failed is not", func(t *testing.T) {
		t.Parallel()

		var (
			tasks    tasksMock.MockTasksRepository
			producer messagingMock.MockProduceConsumer
		)

		tasks.On("GetOne", mock.Anything, taskUUID).Return(failed(time.Now()), nil)
		tasks.On("Save", mock.Anything, mock.Anything).Return(taskUUID, nil)

		require.NoError(t, handler(&tasks, &producer, logsMock.NewInMemoryRepository()).
			Handle(context.Background(), beatOfFailure(t, 2)))

		producer.AssertNotCalled(t, "Produce", mock.Anything, mock.Anything, mock.Anything)
	})
}

func TestHeartbeat_Handle_comingBack(t *testing.T) {
	t.Parallel()

	t.Run("a task the workload gave up on is wanted again once it is running", func(t *testing.T) {
		t.Parallel()

		var (
			tasks    tasksMock.MockTasksRepository
			producer messagingMock.MockProduceConsumer
		)

		// it ran out of attempts, and then came up anyway — a schedule that
		// had been waiting for a node, or somebody starting it by hand.
		given := task.Task{
			UUID:          taskUUID,
			Kind:          task.KindService,
			CurrentState:  task.Failed,
			ExpectedState: task.Failed,
		}

		tasks.On("GetOne", mock.Anything, taskUUID).Return(given, nil)
		tasks.On("Save", mock.Anything, mock.MatchedBy(func(t *task.Task) bool {
			return t.CurrentState == task.Running && t.ExpectedState == task.Running
		})).Return(taskUUID, nil).Once()
		defer tasks.AssertExpectations(t)

		producer.On("Produce", mock.Anything, events.TaskRanName, mock.Anything).Return(nil).Once()

		payload, err := json.Marshal(events.Heartbeat{
			UUID:     taskUUID,
			Kind:     string(task.KindService),
			State:    int(task.Running),
			NodeName: "workload-orchestrator-01",
			At:       time.Now(),
		})
		require.NoError(t, err)

		require.NoError(t, handler(&tasks, &producer, logsMock.NewInMemoryRepository()).
			Handle(context.Background(), payload))
	})
}

// delivering is the control plane's messages as it hears them: each one a
// heartbeat produces is kept, and handed there and then to the handler
// subscribed to it, when there is one here.
type delivering struct {
	messagingMock.Recorder

	handlers map[string]domain.MessageHandler
}

func (d *delivering) Produce(ctx context.Context, subject string, payload []byte) error {
	if err := d.Recorder.Produce(ctx, subject, payload); err != nil {
		return err
	}

	if handler, ok := d.handlers[subject]; ok {
		return handler.Handle(ctx, payload)
	}

	return nil
}

func TestHeartbeat_Handle_started(t *testing.T) {
	t.Parallel()

	started := time.Now().Add(-time.Minute)

	// running is what a node says at the moment at of a job whose run started
	// at started, with 30 s to run.
	running := func(t *testing.T, at time.Time) []byte {
		t.Helper()

		payload, err := json.Marshal(events.Heartbeat{
			UUID:        taskUUID,
			Name:        "a-name",
			Kind:        string(task.KindJob),
			State:       int(task.Running),
			NodeName:    "workload-orchestrator-01",
			ExecutionID: "execution-id",
			StartedAt:   started,
			Deadline:    started.Add(30 * time.Second),
			At:          at,
		})
		require.NoError(t, err)

		return payload
	}

	t.Run("a running task is said to have started when its node says, rather than when it was heard", func(t *testing.T) {
		t.Parallel()

		tasks := tasksMemory.NewRepository(task.Task{UUID: taskUUID, Kind: task.KindJob, CurrentState: task.Scheduled, ExpectedState: task.Running})
		producer := &messagingMock.Recorder{}

		require.NoError(t, handler(tasks, producer, logsMock.NewInMemoryRepository()).Handle(t.Context(), running(t, started.Add(10*time.Second))))

		ran, err := messagingMock.Produced[events.TaskRan](producer, events.TaskRanName)
		require.NoError(t, err)
		require.Len(t, ran, 1)
		assert.True(t, started.Equal(ran[0].StartedAt), "want %s, got %s", started, ran[0].StartedAt)
		assert.True(t, started.Add(30*time.Second).Equal(ran[0].Deadline), "want %s, got %s", started.Add(30*time.Second), ran[0].Deadline)
	})

	// what a job's beats say reaches the rest of the control plane, which
	// writes down when its run started; a beat once its time is up stops it.
	t.Run("a job that outlives its ttl is stopped, counted from when its node says it started", func(t *testing.T) {
		t.Parallel()

		tasks := tasksMemory.NewRepository(task.Task{
			UUID:          taskUUID,
			Kind:          task.KindJob,
			TTL:           30 * time.Second,
			CurrentState:  task.Scheduled,
			ExpectedState: task.Running,
			CreatedAt:     started.Add(-time.Second),
		})

		producer := &delivering{handlers: map[string]domain.MessageHandler{
			events.TaskRanName: runTask.NewTaskRan(tasks),
		}}

		heartbeat := handler(tasks, producer, logsMock.NewInMemoryRepository())

		killed := func() []events.TaskKillRequested {
			kills, err := messagingMock.Produced[events.TaskKillRequested](&producer.Recorder, events.TaskKillRequestedName)
			require.NoError(t, err)

			return kills
		}

		// it comes up, and is written down as having started when its node
		// says it did.
		require.NoError(t, heartbeat.Handle(t.Context(), running(t, started.Add(time.Second))))

		stored, ok := tasks.Stored(taskUUID)
		require.True(t, ok)
		assert.Equal(t, task.Running, stored.CurrentState)
		assert.True(t, started.Equal(stored.StartedAt), "want %s, got %s", started, stored.StartedAt)

		// inside its time, it is left to run.
		require.NoError(t, heartbeat.Handle(t.Context(), running(t, started.Add(29*time.Second))))
		assert.Empty(t, killed())

		// past it, it is stopped.
		require.NoError(t, heartbeat.Handle(t.Context(), running(t, started.Add(31*time.Second))))
		assert.Equal(t, []events.TaskKillRequested{{UUID: taskUUID}}, killed())

		stored, _ = tasks.Stored(taskUUID)
		assert.Equal(t, task.Stopping, stored.CurrentState)
		assert.Equal(t, task.Stopped, stored.ExpectedState)
	})
}
