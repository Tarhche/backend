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

	"github.com/khanzadimahdi/testproject/domain/runner/container"
	"github.com/khanzadimahdi/testproject/domain/runner/task/events"
	messagingMock "github.com/khanzadimahdi/testproject/infrastructure/messaging/mock"
	containersMock "github.com/khanzadimahdi/testproject/infrastructure/repository/mocks/runner/containers"
)

const nodeName = "node-1"

func discardLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

// heldContainer is one running container on this node, as docker lists it.
func heldContainer(labels map[string]string) container.Container {
	all := map[string]string{
		container.TaskUUIDLabelKey: "task-uuid",
		container.TaskNameLabelKey: "task-name",
		container.TaskKindLabelKey: "service",
		container.NodeNameLabelKey: nodeName,
	}

	for key, value := range labels {
		all[key] = value
	}

	return container.Container{
		ID:     "container-id",
		Name:   "/task-name",
		Status: container.StatusRunning,
		Image:  "nginx:1.27-alpine",
		Labels: all,
	}
}

// beaten is the deadline the node reported for the only container it holds.
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

	t.Run("counts a container's time from when it started running", func(t *testing.T) {
		t.Parallel()

		var (
			manager  containersMock.MockContainerManager
			producer messagingMock.MockProduceConsumer
		)

		held := heldContainer(map[string]string{container.TaskTTLLabelKey: "120"})

		manager.On("GetByLabel", mock.Anything, container.NodeNameLabelKey, nodeName).
			Return([]container.Container{held}, nil)
		manager.On("Inspect", mock.Anything, held.ID).Once().
			Return(container.Container{ID: held.ID, StartedAt: started}, nil)
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

	t.Run("a container that may run as long as it likes has no deadline", func(t *testing.T) {
		t.Parallel()

		var (
			manager  containersMock.MockContainerManager
			producer messagingMock.MockProduceConsumer
		)

		manager.On("GetByLabel", mock.Anything, container.NodeNameLabelKey, nodeName).
			Return([]container.Container{heldContainer(nil)}, nil)
		producer.On("Produce", mock.Anything, events.HeartbeatName, mock.Anything).Return(nil)

		useCase := NewUseCase(&manager, &producer, nodeName, discardLogger())

		require.NoError(t, useCase.Execute(context.Background()))
		assert.True(t, beaten(t, &producer).IsZero())
		manager.AssertNotCalled(t, "Inspect", mock.Anything, mock.Anything)
	})

	t.Run("a container that has not started yet counts down to nothing", func(t *testing.T) {
		t.Parallel()

		var (
			manager  containersMock.MockContainerManager
			producer messagingMock.MockProduceConsumer
		)

		held := heldContainer(map[string]string{container.TaskTTLLabelKey: "120"})

		manager.On("GetByLabel", mock.Anything, container.NodeNameLabelKey, nodeName).
			Return([]container.Container{held}, nil)
		manager.On("Inspect", mock.Anything, held.ID).
			Return(container.Container{ID: held.ID}, nil)
		producer.On("Produce", mock.Anything, events.HeartbeatName, mock.Anything).Return(nil)

		useCase := NewUseCase(&manager, &producer, nodeName, discardLogger())

		require.NoError(t, useCase.Execute(context.Background()))
		assert.True(t, beaten(t, &producer).IsZero())
	})
}
