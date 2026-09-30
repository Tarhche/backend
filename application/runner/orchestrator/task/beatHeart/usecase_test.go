package beatHeart

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

	"github.com/khanzadimahdi/testproject/domain/runner/task"
	"github.com/khanzadimahdi/testproject/domain/runner/task/events"
	messagingMock "github.com/khanzadimahdi/testproject/infrastructure/messaging/mock"
	runtimeMock "github.com/khanzadimahdi/testproject/infrastructure/repository/mocks/runner/runtime"
)

const nodeName = "node-1"

func discardLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

// heldTask is one running task on this node, as the runtime reports it.
func heldTask(adjust ...func(*task.Execution)) task.Execution {
	held := task.Execution{
		ID:       "task-id",
		Name:     "/task-name",
		Status:   task.StatusRunning,
		Image:    "nginx:1.27-alpine",
		TaskUUID: "task-uuid",
		TaskName: "task-name",
		Kind:     task.KindService,
		NodeName: nodeName,
	}

	for _, change := range adjust {
		change(&held)
	}

	return held
}

// allowed is how long the task may run for once it is up.
func allowed(ttl time.Duration) func(*task.Execution) {
	return func(held *task.Execution) { held.TTL = ttl }
}

// asJob makes it one that is expected to exit.
func asJob() func(*task.Execution) {
	return func(held *task.Execution) { held.Kind = task.KindJob }
}

// beaten is the deadline the node reported for the only task it holds.
func beaten(t *testing.T, producer *messagingMock.MockProduceConsumer) time.Time {
	t.Helper()

	require.NotEmpty(t, producer.Calls)

	var heartbeat events.Heartbeat
	require.NoError(t, json.Unmarshal(producer.Calls[0].Arguments[2].([]byte), &heartbeat))

	return heartbeat.Deadline
}

func TestUseCase_Execute_deadline(t *testing.T) {
	t.Parallel()

	started := time.Now().Add(-30 * time.Second).Truncate(time.Millisecond)

	t.Run("counts a task's time from when it started running", func(t *testing.T) {
		t.Parallel()

		var (
			manager  runtimeMock.MockRuntime
			producer messagingMock.MockProduceConsumer
		)

		held := heldTask(allowed(2 * time.Minute))

		manager.On("OnNode", mock.Anything, nodeName).
			Return([]task.Execution{held}, nil)
		manager.On("Inspect", mock.Anything, held.ID).Once().
			Return(task.Execution{ID: held.ID, StartedAt: started}, nil)
		producer.On("Produce", mock.Anything, events.HeartbeatName, mock.Anything).Return(nil)

		useCase := NewUseCase(&manager, &producer, nodeName, discardLogger())

		require.NoError(t, useCase.Execute(context.Background()))

		// compared as an instant rather than as a value: the same moment read
		// back through json carries UTC, and a machine whose clock is set to
		// anything else would otherwise disagree with itself.
		assert.True(t, started.Add(2*time.Minute).Equal(beaten(t, &producer)),
			"want %s, got %s", started.Add(2*time.Minute), beaten(t, &producer))

		// a second beat asks docker nothing: when it started does not change.
		require.NoError(t, useCase.Execute(context.Background()))
		manager.AssertNumberOfCalls(t, "Inspect", 1)
	})

	t.Run("a task that may run as long as it likes has no deadline", func(t *testing.T) {
		t.Parallel()

		var (
			manager  runtimeMock.MockRuntime
			producer messagingMock.MockProduceConsumer
		)

		manager.On("OnNode", mock.Anything, nodeName).
			Return([]task.Execution{heldTask()}, nil)
		producer.On("Produce", mock.Anything, events.HeartbeatName, mock.Anything).Return(nil)

		useCase := NewUseCase(&manager, &producer, nodeName, discardLogger())

		require.NoError(t, useCase.Execute(context.Background()))
		assert.True(t, beaten(t, &producer).IsZero())
		manager.AssertNotCalled(t, "Inspect", mock.Anything, mock.Anything)
	})

	t.Run("a task that has not started yet counts down to nothing", func(t *testing.T) {
		t.Parallel()

		var (
			manager  runtimeMock.MockRuntime
			producer messagingMock.MockProduceConsumer
		)

		held := heldTask(allowed(2 * time.Minute))

		manager.On("OnNode", mock.Anything, nodeName).
			Return([]task.Execution{held}, nil)
		manager.On("Inspect", mock.Anything, held.ID).
			Return(task.Execution{ID: held.ID}, nil)
		producer.On("Produce", mock.Anything, events.HeartbeatName, mock.Anything).Return(nil)

		useCase := NewUseCase(&manager, &producer, nodeName, discardLogger())

		require.NoError(t, useCase.Execute(context.Background()))
		assert.True(t, beaten(t, &producer).IsZero())
	})
}

// reportedState is the state the node reported for the only task it holds.
func reportedState(t *testing.T, producer *messagingMock.MockProduceConsumer) task.State {
	t.Helper()

	require.NotEmpty(t, producer.Calls)

	var heartbeat events.Heartbeat
	require.NoError(t, json.Unmarshal(producer.Calls[0].Arguments[2].([]byte), &heartbeat))

	return task.State(heartbeat.State)
}

func TestUseCase_Execute_exitCode(t *testing.T) {
	t.Parallel()

	ended := func(exitCode int) task.Execution {
		c := heldTask(asJob())
		c.Status = task.StatusExited
		c.ExitCode = exitCode

		return c
	}

	t.Run("a job that returned a failure did not complete", func(t *testing.T) {
		t.Parallel()

		var (
			manager  runtimeMock.MockRuntime
			producer messagingMock.MockProduceConsumer
		)

		held := ended(0)

		manager.On("OnNode", mock.Anything, nodeName).
			Return([]task.Execution{held}, nil)
		manager.On("Inspect", mock.Anything, held.ID).Once().
			Return(ended(3), nil)
		manager.On("Logs", mock.Anything, held.ID, mock.Anything).Return(nil)
		producer.On("Produce", mock.Anything, events.HeartbeatName, mock.Anything).Return(nil)

		useCase := NewUseCase(&manager, &producer, nodeName, discardLogger())

		require.NoError(t, useCase.Execute(context.Background()))
		assert.Equal(t, task.Failed, reportedState(t, &producer))

		// a second beat asks docker nothing: what it returned does not change.
		require.NoError(t, useCase.Execute(context.Background()))
		manager.AssertNumberOfCalls(t, "Inspect", 1)
	})

	t.Run("a job ended by a signal completed", func(t *testing.T) {
		t.Parallel()

		var (
			manager  runtimeMock.MockRuntime
			producer messagingMock.MockProduceConsumer
		)

		held := ended(0)

		manager.On("OnNode", mock.Anything, nodeName).
			Return([]task.Execution{held}, nil)
		manager.On("Inspect", mock.Anything, held.ID).
			Return(ended(137), nil)
		manager.On("Logs", mock.Anything, held.ID, mock.Anything).Return(nil)
		producer.On("Produce", mock.Anything, events.HeartbeatName, mock.Anything).Return(nil)

		useCase := NewUseCase(&manager, &producer, nodeName, discardLogger())

		require.NoError(t, useCase.Execute(context.Background()))
		assert.Equal(t, task.Completed, reportedState(t, &producer))
	})

	t.Run("a task that is still running is not asked what it returned", func(t *testing.T) {
		t.Parallel()

		var (
			manager  runtimeMock.MockRuntime
			producer messagingMock.MockProduceConsumer
		)

		held := heldTask(asJob())

		manager.On("OnNode", mock.Anything, nodeName).
			Return([]task.Execution{held}, nil)
		manager.On("Logs", mock.Anything, held.ID, mock.Anything).Return(nil)
		producer.On("Produce", mock.Anything, events.HeartbeatName, mock.Anything).Return(nil)

		useCase := NewUseCase(&manager, &producer, nodeName, discardLogger())

		require.NoError(t, useCase.Execute(context.Background()))
		assert.Equal(t, task.Running, reportedState(t, &producer))
		manager.AssertNotCalled(t, "Inspect", mock.Anything, mock.Anything)
	})
}
