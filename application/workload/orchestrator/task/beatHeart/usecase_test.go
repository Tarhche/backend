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

	"github.com/khanzadimahdi/testproject/domain/workload/task"
	"github.com/khanzadimahdi/testproject/domain/workload/task/events"
	messagingMock "github.com/khanzadimahdi/testproject/infrastructure/messaging/mock"
	runtimeMock "github.com/khanzadimahdi/testproject/infrastructure/repository/mocks/workload/runtime"
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

// since makes it one whose run started at started, as the runtime lists it.
func since(started time.Time) func(*task.Execution) {
	return func(held *task.Execution) { held.StartedAt = started }
}

// beaten is what the node said the nth time it beat for the only task it
// holds.
func beaten(t *testing.T, producer *messagingMock.MockProduceConsumer, n int) events.Heartbeat {
	t.Helper()

	require.Greater(t, len(producer.Calls), n)

	var heartbeat events.Heartbeat
	require.NoError(t, json.Unmarshal(producer.Calls[n].Arguments[2].([]byte), &heartbeat))

	return heartbeat
}

// sameInstant asserts two times are the same moment. They are compared as
// instants rather than as values: a moment read back through json carries
// UTC and no monotonic reading, and a machine whose clock is set to anything
// else would otherwise disagree with itself.
func sameInstant(t *testing.T, want time.Time, got time.Time, what string) {
	t.Helper()

	assert.True(t, want.Equal(got), "%s: want %s, got %s", what, want, got)
}

func TestUseCase_Execute_started(t *testing.T) {
	t.Parallel()

	started := time.Now().Add(-30 * time.Second)

	t.Run("a task says when its run started, and counts its time from then", func(t *testing.T) {
		t.Parallel()

		var (
			manager  runtimeMock.MockRuntime
			producer messagingMock.MockProduceConsumer
		)

		manager.On("OnNode", mock.Anything, nodeName).
			Return([]task.Execution{heldTask(asJob(), allowed(2*time.Minute), since(started))}, nil)
		manager.On("Logs", mock.Anything, "task-id", mock.Anything).Return(nil)
		producer.On("Produce", mock.Anything, events.HeartbeatName, mock.Anything).Return(nil)

		require.NoError(t, NewUseCase(&manager, &producer, nodeName, discardLogger()).Execute(context.Background()))

		beat := beaten(t, &producer, 0)
		sameInstant(t, started, beat.StartedAt, "started")
		sameInstant(t, started.Add(2*time.Minute), beat.Deadline, "deadline")

		// the runtime says it of every run it lists: nothing more is asked.
		manager.AssertNotCalled(t, "Inspect", mock.Anything, mock.Anything)
	})

	t.Run("a run that started again is counted from then", func(t *testing.T) {
		t.Parallel()

		var (
			manager  runtimeMock.MockRuntime
			producer messagingMock.MockProduceConsumer
		)

		again := started.Add(20 * time.Second)

		manager.On("OnNode", mock.Anything, nodeName).Once().
			Return([]task.Execution{heldTask(asJob(), allowed(2*time.Minute), since(started))}, nil)
		manager.On("OnNode", mock.Anything, nodeName).Once().
			Return([]task.Execution{heldTask(asJob(), allowed(2*time.Minute), since(again))}, nil)
		manager.On("Logs", mock.Anything, "task-id", mock.Anything).Return(nil)
		producer.On("Produce", mock.Anything, events.HeartbeatName, mock.Anything).Return(nil)

		useCase := NewUseCase(&manager, &producer, nodeName, discardLogger())

		require.NoError(t, useCase.Execute(context.Background()))
		require.NoError(t, useCase.Execute(context.Background()))

		beat := beaten(t, &producer, 1)
		sameInstant(t, again, beat.StartedAt, "started")
		sameInstant(t, again.Add(2*time.Minute), beat.Deadline, "deadline")
	})

	t.Run("a task that may run as long as it likes says when it started, and has no deadline", func(t *testing.T) {
		t.Parallel()

		var (
			manager  runtimeMock.MockRuntime
			producer messagingMock.MockProduceConsumer
		)

		manager.On("OnNode", mock.Anything, nodeName).
			Return([]task.Execution{heldTask(since(started))}, nil)
		producer.On("Produce", mock.Anything, events.HeartbeatName, mock.Anything).Return(nil)

		require.NoError(t, NewUseCase(&manager, &producer, nodeName, discardLogger()).Execute(context.Background()))

		beat := beaten(t, &producer, 0)
		sameInstant(t, started, beat.StartedAt, "started")
		assert.True(t, beat.Deadline.IsZero())
	})

	t.Run("a task that has not started yet counts down to nothing", func(t *testing.T) {
		t.Parallel()

		var (
			manager  runtimeMock.MockRuntime
			producer messagingMock.MockProduceConsumer
		)

		manager.On("OnNode", mock.Anything, nodeName).
			Return([]task.Execution{heldTask(asJob(), allowed(2*time.Minute))}, nil)
		manager.On("Logs", mock.Anything, "task-id", mock.Anything).Return(nil)
		producer.On("Produce", mock.Anything, events.HeartbeatName, mock.Anything).Return(nil)

		require.NoError(t, NewUseCase(&manager, &producer, nodeName, discardLogger()).Execute(context.Background()))

		beat := beaten(t, &producer, 0)
		assert.True(t, beat.StartedAt.IsZero())
		assert.True(t, beat.Deadline.IsZero())
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
