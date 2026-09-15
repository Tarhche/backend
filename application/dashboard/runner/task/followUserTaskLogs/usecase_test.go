package followUserTaskLogs

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

	"github.com/khanzadimahdi/testproject/application/auth"
	"github.com/khanzadimahdi/testproject/application/dashboard/runner/logs"
	"github.com/khanzadimahdi/testproject/domain"
	"github.com/khanzadimahdi/testproject/domain/runner/task"
	"github.com/khanzadimahdi/testproject/domain/user"
	messagingMock "github.com/khanzadimahdi/testproject/infrastructure/messaging/mock"
	runnerMock "github.com/khanzadimahdi/testproject/infrastructure/repository/mocks/runner/manager"
	"github.com/khanzadimahdi/testproject/infrastructure/validator"
	"github.com/khanzadimahdi/testproject/infrastructure/websocket/gateway"
)

const (
	taskUUID  = "task-uuid"
	userUUID  = "user-uuid"
	requestID = "server-side-request-id"
)

var written = time.Date(2026, 9, 15, 9, 0, 0, 0, time.UTC)

func discardLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

func accepts() *validator.MockValidator {
	v := &validator.MockValidator{}
	v.On("Validate", mock.Anything).Return(domain.ValidationErrors{})

	return v
}

// asking is the context a request arrives in: whoever the middleware
// established before it got here.
func asking() context.Context {
	return auth.ToContext(context.Background(), &user.User{UUID: userUUID})
}

func request(t *testing.T) []byte {
	t.Helper()

	payload, err := json.Marshal(Request{ID: requestID, TaskUUID: taskUUID})
	require.NoError(t, err)

	return payload
}

func TestUseCase_Handle(t *testing.T) {
	t.Parallel()

	t.Run("a task that is theirs is followed", func(t *testing.T) {
		t.Parallel()

		var (
			runner  runnerMock.MockClient
			replyer messagingMock.RecordingReplyer
		)

		runner.On("TaskOf", mock.Anything, userUUID, taskUUID).
			Return(task.Task{UUID: taskUUID, OwnerUUID: userUUID}, nil).Once()
		runner.On("TaskLogs", mock.Anything, taskUUID, time.Time{}, backlog).Return([]task.Log{
			{TaskUUID: taskUUID, LogLine: task.LogLine{Stream: task.StreamStdout, Content: "listening on :80", At: written}},
		}, nil).Once()
		defer runner.AssertExpectations(t)

		followers := logs.NewFollowers(&replyer, discardLogger())

		useCase := NewUseCase(&runner, followers, accepts(), &replyer, gateway.NewStreams(), discardLogger())
		require.NoError(t, useCase.Handle(asking(), request(t)))

		require.Len(t, replyer.Chunks(), 1)
		assert.Equal(t, 1, followers.Len())

		var line logs.Response
		require.NoError(t, json.Unmarshal(replyer.Chunks()[0], &line))
		assert.Equal(t, "listening on :80", line.Content)
	})

	t.Run("a task that is somebody else's is not there for them", func(t *testing.T) {
		t.Parallel()

		var (
			runner  runnerMock.MockClient
			replyer messagingMock.RecordingReplyer
		)

		runner.On("TaskOf", mock.Anything, userUUID, taskUUID).
			Return(task.Task{}, domain.ErrNotExists).Once()

		followers := logs.NewFollowers(&replyer, discardLogger())

		useCase := NewUseCase(&runner, followers, accepts(), &replyer, gateway.NewStreams(), discardLogger())
		require.NoError(t, useCase.Handle(asking(), request(t)))

		replies := replyer.Replies()
		require.Len(t, replies, 1)
		assert.Equal(t, domain.ReplyEOF, replies[0].Kind)
		assert.Zero(t, followers.Len())

		runner.AssertNotCalled(t, "TaskLogs", mock.Anything, mock.Anything, mock.Anything, mock.Anything)
	})

	t.Run("a malformed request is dropped rather than redelivered", func(t *testing.T) {
		t.Parallel()

		var (
			runner  runnerMock.MockClient
			replyer messagingMock.RecordingReplyer
		)

		useCase := NewUseCase(&runner, logs.NewFollowers(&replyer, discardLogger()), accepts(), &replyer, gateway.NewStreams(), discardLogger())

		assert.NoError(t, useCase.Handle(asking(), []byte("{")))
		assert.Empty(t, replyer.Replies())
	})
}
