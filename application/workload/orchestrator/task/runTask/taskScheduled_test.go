package runTask

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	workloadRuntime "github.com/khanzadimahdi/testproject/domain/workload/runtime"
	"github.com/khanzadimahdi/testproject/domain/workload/task"
	"github.com/khanzadimahdi/testproject/domain/workload/task/events"
	messagingMock "github.com/khanzadimahdi/testproject/infrastructure/messaging/mock"
	"github.com/khanzadimahdi/testproject/infrastructure/repository/mocks/workload/runtime"
)

func scheduled(t *testing.T) []byte {
	t.Helper()

	payload, err := json.Marshal(events.TaskScheduled{
		UUID:          "task-uuid",
		Name:          "a-request-id",
		Image:         "ghcr.io/example/workload:latest",
		NominatedNode: nodeName,
	})
	require.NoError(t, err)

	return payload
}

func discardLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

func TestTaskScheduled_Handle(t *testing.T) {
	t.Parallel()

	t.Run("a task that cannot be started is reported as failed, with the reason", func(t *testing.T) {
		t.Parallel()

		var (
			taskManager    runtime.MockRuntime
			networkManager runtime.MockNetworkManager
			producer       messagingMock.MockProduceConsumer
		)

		networkManager.On("EnsureIsolatedNetwork", mock.Anything).Return(nil)
		taskManager.On("EnsureImage", mock.Anything, mock.Anything).Return(nil)
		taskManager.On("Create", mock.Anything, mock.Anything).
			Return("", errors.New("no such image: ghcr.io/example/workload:latest")).Once()

		// nothing is there before it runs, and nothing was created, so there is
		// no task to take instead either.
		taskManager.On("Of", mock.Anything, mock.Anything).
			Return([]task.Execution{}, nil).Twice()
		producer.On("Produce", mock.Anything, events.TaskFailedName, mock.Anything).Return(nil).Once()
		defer producer.AssertExpectations(t)

		useCase := NewUseCase(sysboxOnly(&taskManager, &networkManager), accepts(), nodeName)

		// no error: the failure is announced rather than handed back, which is
		// what would have the message delivered again.
		require.NoError(t, NewTaskScheduled(useCase, &producer, nodeName, discardLogger()).
			Handle(context.Background(), scheduled(t)))

		var failed events.TaskFailed
		require.NoError(t, json.Unmarshal(producer.Calls[0].Arguments.Get(2).([]byte), &failed))

		assert.Equal(t, "task-uuid", failed.UUID)
		assert.Equal(t, "a-request-id", failed.Name, "so whoever asked for it can be told")
		assert.Equal(t, nodeName, failed.NodeName)
		assert.Contains(t, failed.Reason, "no such image")
	})

	t.Run("a task nominated for another node is left to it", func(t *testing.T) {
		t.Parallel()

		var (
			taskManager    runtime.MockRuntime
			networkManager runtime.MockNetworkManager
			producer       messagingMock.MockProduceConsumer
		)

		useCase := NewUseCase(sysboxOnly(&taskManager, &networkManager), accepts(), nodeName)

		require.NoError(t, NewTaskScheduled(useCase, &producer, "workload-orchestrator-99", discardLogger()).
			Handle(context.Background(), scheduled(t)))

		taskManager.AssertNotCalled(t, "Create", mock.Anything, mock.Anything)
		producer.AssertNotCalled(t, "Produce", mock.Anything, mock.Anything, mock.Anything)
	})
}

func TestTaskScheduled_Handle_classes(t *testing.T) {
	t.Parallel()

	t.Run("a class this node does not offer fails the task with the reason's code, and is not handed back", func(t *testing.T) {
		t.Parallel()

		var (
			taskManager    runtime.MockRuntime
			networkManager runtime.MockNetworkManager
			producer       messagingMock.MockProduceConsumer
		)

		producer.On("Produce", mock.Anything, events.TaskFailedName, mock.Anything).Return(nil).Once()
		defer producer.AssertExpectations(t)

		payload, err := json.Marshal(events.TaskScheduled{
			UUID:          "task-uuid",
			Name:          "a-request-id",
			Runtime:       workloadRuntime.Firecracker,
			Image:         "ghcr.io/example/workload:latest",
			NominatedNode: nodeName,
			Attempt:       1,
			MaxRetries:    3,
		})
		require.NoError(t, err)

		useCase := NewUseCase(sysboxOnly(&taskManager, &networkManager), accepts(), nodeName)

		// no error: returning one would have the message delivered again, to a
		// node that will never offer the class.
		require.NoError(t, NewTaskScheduled(useCase, &producer, nodeName, discardLogger()).
			Handle(context.Background(), payload))

		var failed events.TaskFailed
		require.NoError(t, json.Unmarshal(producer.Calls[0].Arguments.Get(2).([]byte), &failed))

		// a code, as it is, which the dashboard says in the reader's language.
		assert.Equal(t, workloadRuntime.ReasonRuntimeNotOffered, failed.Reason)
		assert.Equal(t, "task-uuid", failed.UUID)
		assert.Equal(t, nodeName, failed.NodeName)
		assert.Equal(t, 1, failed.Attempt)
		assert.Equal(t, 3, failed.MaxRetries)

		taskManager.AssertNotCalled(t, "Create", mock.Anything, mock.Anything)
	})

	t.Run("a control plane from before there were classes asks for sysbox", func(t *testing.T) {
		t.Parallel()

		var (
			taskManager    runtime.MockRuntime
			networkManager runtime.MockNetworkManager
			producer       messagingMock.MockProduceConsumer
		)

		networkManager.On("EnsureIsolatedNetwork", mock.Anything).Return(nil).Once()
		taskManager.On("Of", mock.Anything, "task-uuid").Return([]task.Execution{}, nil).Once()
		taskManager.On("EnsureImage", mock.Anything, mock.Anything).Return(nil).Once()
		taskManager.On("Create", mock.Anything, mock.Anything).Return("4f2c9d0b7a1e", nil).Once()
		taskManager.On("Start", mock.Anything, "4f2c9d0b7a1e").Return(nil).Once()
		defer taskManager.AssertExpectations(t)

		// scheduled(t) names no class, as an older control plane's message does.
		useCase := NewUseCase(sysboxOnly(&taskManager, &networkManager), accepts(), nodeName)

		require.NoError(t, NewTaskScheduled(useCase, &producer, nodeName, discardLogger()).
			Handle(context.Background(), scheduled(t)))

		producer.AssertNotCalled(t, "Produce", mock.Anything, mock.Anything, mock.Anything)
	})
}
