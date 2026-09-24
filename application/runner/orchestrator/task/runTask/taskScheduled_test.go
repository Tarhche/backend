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

	"github.com/khanzadimahdi/testproject/domain/runner/task"
	"github.com/khanzadimahdi/testproject/domain/runner/task/events"
	messagingMock "github.com/khanzadimahdi/testproject/infrastructure/messaging/mock"
	"github.com/khanzadimahdi/testproject/infrastructure/repository/mocks/runner/runtime"
)

func scheduled(t *testing.T) []byte {
	t.Helper()

	payload, err := json.Marshal(events.TaskScheduled{
		UUID:          "task-uuid",
		Name:          "a-request-id",
		Image:         "ghcr.io/example/runner:latest",
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
			Return("", errors.New("no such image: ghcr.io/example/runner:latest")).Once()

		// nothing is there before it runs, and nothing was created, so there is
		// no task to take instead either.
		taskManager.On("Of", mock.Anything, mock.Anything).
			Return([]task.Execution{}, nil).Twice()
		producer.On("Produce", mock.Anything, events.TaskFailedName, mock.Anything).Return(nil).Once()
		defer producer.AssertExpectations(t)

		useCase := NewUseCase(&taskManager, &networkManager, accepts(), nodeName)

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

	t.Run("a task is created as its owner's, so its node can say who may open a terminal in it", func(t *testing.T) {
		t.Parallel()

		var (
			taskManager    runtime.MockRuntime
			networkManager runtime.MockNetworkManager
			producer       messagingMock.MockProduceConsumer
		)

		networkManager.On("EnsureIsolatedNetwork", mock.Anything).Return(nil).Maybe()

		var created *task.Execution
		taskManager.On("Of", mock.Anything, mock.Anything).Return([]task.Execution{}, nil).Maybe()
		taskManager.On("EnsureImage", mock.Anything, mock.Anything).Return(nil).Once()
		taskManager.On("Create", mock.Anything, mock.Anything).
			Run(func(args mock.Arguments) { created = args.Get(1).(*task.Execution) }).
			Return("task-id", nil).Once()
		taskManager.On("Start", mock.Anything, "task-id").Return(nil).Once()
		defer taskManager.AssertExpectations(t)

		payload, err := json.Marshal(events.TaskScheduled{
			UUID:          "task-uuid",
			Name:          "a-request-id",
			Image:         "ghcr.io/example/runner:latest",
			NominatedNode: nodeName,
			OwnerUUID:     "owner-uuid",
		})
		require.NoError(t, err)

		useCase := NewUseCase(&taskManager, &networkManager, accepts(), nodeName)

		require.NoError(t, NewTaskScheduled(useCase, &producer, nodeName, discardLogger()).
			Handle(context.Background(), payload))

		require.NotNil(t, created)
		assert.Equal(t, "owner-uuid", created.OwnerUUID)
		producer.AssertNotCalled(t, "Produce", mock.Anything, mock.Anything, mock.Anything)
	})

	t.Run("a task nominated for another node is left to it", func(t *testing.T) {
		t.Parallel()

		var (
			taskManager    runtime.MockRuntime
			networkManager runtime.MockNetworkManager
			producer       messagingMock.MockProduceConsumer
		)

		useCase := NewUseCase(&taskManager, &networkManager, accepts(), nodeName)

		require.NoError(t, NewTaskScheduled(useCase, &producer, "runner-orchestrator-99", discardLogger()).
			Handle(context.Background(), scheduled(t)))

		taskManager.AssertNotCalled(t, "Create", mock.Anything, mock.Anything)
		producer.AssertNotCalled(t, "Produce", mock.Anything, mock.Anything, mock.Anything)
	})
}
