package stop

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/khanzadimahdi/testproject/application/code/runCode"
	"github.com/khanzadimahdi/testproject/domain"
	"github.com/khanzadimahdi/testproject/domain/workload/kind"
	taskKind "github.com/khanzadimahdi/testproject/domain/workload/kinds/task"
	"github.com/khanzadimahdi/testproject/domain/workload/task"
	messagingMock "github.com/khanzadimahdi/testproject/infrastructure/messaging/mock"
	workloadMock "github.com/khanzadimahdi/testproject/infrastructure/repository/mocks/workload/controlplane"
	"github.com/khanzadimahdi/testproject/infrastructure/validator"
)

const taskUUID = "task-uuid"

func discardLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

func accepts() *validator.MockValidator {
	v := &validator.MockValidator{}
	v.On("Validate", mock.Anything).Return(domain.ValidationErrors{})

	return v
}

func request(t *testing.T) []byte {
	t.Helper()

	payload, err := json.Marshal(Request{ID: "request-id", TaskUUID: taskUUID})
	require.NoError(t, err)

	return payload
}

// held is a task the workload holds, of a kind, whose it is.
func held(k task.Kind, ownerUUID string) taskKind.Task {
	return taskKind.Task{
		Kind:     taskKind.Name,
		Metadata: kind.Metadata{UUID: taskUUID, OwnerUUID: ownerUUID},
		Spec:     taskKind.Spec{Kind: k},
	}
}

// refusal is what the client was told about a task it does not get.
func refusal(t *testing.T, replies []domain.Reply) map[string]string {
	t.Helper()

	require.Len(t, replies, 1)

	var body struct {
		Errors map[string]string `json:"errors"`
	}
	require.NoError(t, json.Unmarshal(replies[0].Payload, &body))

	return body.Errors
}

func TestUseCase_Handle(t *testing.T) {
	t.Parallel()

	t.Run("takes away the task a snippet is running in", func(t *testing.T) {
		t.Parallel()

		var (
			workload workloadMock.MockClient
			replyer  messagingMock.RecordingReplyer
		)

		workload.On("Task", mock.Anything, taskUUID).Once().
			Return(held(task.KindJob, runCode.CodeRunnerOwnerUUID), nil)
		workload.On("DeleteTask", mock.Anything, taskUUID).Once().Return(nil)
		defer workload.AssertExpectations(t)

		require.NoError(t, NewUseCase(&workload, accepts(), &replyer, discardLogger()).
			Handle(context.Background(), request(t)))

		assert.Empty(t, refusal(t, replyer.Replies()), "a task that is gone is nothing to report")
	})

	t.Run("a task the code runner does not own is not there to stop", func(t *testing.T) {
		t.Parallel()

		var (
			workload workloadMock.MockClient
			replyer  messagingMock.RecordingReplyer
		)

		// somebody's own task from the dashboard: naming it here does not
		// make it a snippet's.
		workload.On("Task", mock.Anything, taskUUID).Once().
			Return(held(task.KindService, "somebody"), nil)
		defer workload.AssertExpectations(t)

		require.NoError(t, NewUseCase(&workload, accepts(), &replyer, discardLogger()).
			Handle(context.Background(), request(t)))

		assert.Equal(t, "not_exists", refusal(t, replyer.Replies())["task_uuid"])
		workload.AssertNotCalled(t, "DeleteTask", mock.Anything, mock.Anything)
	})

	t.Run("a job that belongs to somebody is not a snippet's either", func(t *testing.T) {
		t.Parallel()

		var (
			workload workloadMock.MockClient
			replyer  messagingMock.RecordingReplyer
		)

		workload.On("Task", mock.Anything, taskUUID).Once().
			Return(held(task.KindJob, "somebody"), nil)
		defer workload.AssertExpectations(t)

		require.NoError(t, NewUseCase(&workload, accepts(), &replyer, discardLogger()).
			Handle(context.Background(), request(t)))

		assert.Equal(t, "not_exists", refusal(t, replyer.Replies())["task_uuid"])
		workload.AssertNotCalled(t, "DeleteTask", mock.Anything, mock.Anything)
	})

	t.Run("a task that is already gone is what was asked for", func(t *testing.T) {
		t.Parallel()

		var (
			workload workloadMock.MockClient
			replyer  messagingMock.RecordingReplyer
		)

		workload.On("Task", mock.Anything, taskUUID).Once().Return(taskKind.Task{}, domain.ErrNotExists)
		defer workload.AssertExpectations(t)

		require.NoError(t, NewUseCase(&workload, accepts(), &replyer, discardLogger()).
			Handle(context.Background(), request(t)))

		assert.Empty(t, refusal(t, replyer.Replies()))
		workload.AssertNotCalled(t, "DeleteTask", mock.Anything, mock.Anything)
	})
}
