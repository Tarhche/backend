package followTaskLogs

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

	"github.com/khanzadimahdi/testproject/application/dashboard/runner/logs"
	"github.com/khanzadimahdi/testproject/domain"
	"github.com/khanzadimahdi/testproject/domain/runner/task"
	taskEvents "github.com/khanzadimahdi/testproject/domain/runner/task/events"
	messagingMock "github.com/khanzadimahdi/testproject/infrastructure/messaging/mock"
	runnerMock "github.com/khanzadimahdi/testproject/infrastructure/repository/mocks/runner/controlplane"
	"github.com/khanzadimahdi/testproject/infrastructure/validator"
	"github.com/khanzadimahdi/testproject/infrastructure/websocket/gateway"
)

const (
	taskUUID  = "task-uuid"
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

func request(t *testing.T, after time.Time) []byte {
	t.Helper()

	payload, err := json.Marshal(Request{
		ID:       requestID,
		TaskUUID: taskUUID,
		After:    after,
	})
	require.NoError(t, err)

	return payload
}

// said is the content of every line sent to the client, in order.
func said(t *testing.T, replyer *messagingMock.RecordingReplyer) []string {
	t.Helper()

	chunks := replyer.Chunks()
	lines := make([]string, len(chunks))

	for i, chunk := range chunks {
		var line logs.Response
		require.NoError(t, json.Unmarshal(chunk, &line))

		lines[i] = line.Content
	}

	return lines
}

func TestUseCase_Handle(t *testing.T) {
	t.Parallel()

	t.Run("a stream opens with what the task has already written", func(t *testing.T) {
		t.Parallel()

		var (
			runner  runnerMock.MockClient
			replyer messagingMock.RecordingReplyer
		)

		runner.On("TaskLogs", mock.Anything, taskUUID, time.Time{}, backlog).Return([]task.Log{
			{TaskUUID: taskUUID, LogLine: task.LogLine{Stream: task.StreamStdout, Content: "listening on :80", At: written}},
		}, nil).Once()
		defer runner.AssertExpectations(t)

		followers := logs.NewFollowers(&replyer, discardLogger())

		useCase := NewUseCase(&runner, followers, accepts(), &replyer, gateway.NewStreams(), discardLogger())
		require.NoError(t, useCase.Handle(context.Background(), request(t, time.Time{})))

		assert.Equal(t, []string{"listening on :80"}, said(t, &replyer))

		// and then whatever it writes from here on.
		logged, err := json.Marshal(taskEvents.TaskLogged{
			UUID:  taskUUID,
			Lines: []taskEvents.LogLine{{Content: "a request", At: written.Add(time.Second)}},
		})
		require.NoError(t, err)
		require.NoError(t, followers.Lines(context.Background(), logged))

		assert.Equal(t, []string{"listening on :80", "a request"}, said(t, &replyer))
	})

	t.Run("it picks up from where the reader had already caught up to", func(t *testing.T) {
		t.Parallel()

		var (
			runner  runnerMock.MockClient
			replyer messagingMock.RecordingReplyer
		)

		// the page is rendered with what a task has already written, so
		// the stream starts after the last of those and nothing is shown twice.
		runner.On("TaskLogs", mock.Anything, taskUUID, written, backlog).Return([]task.Log{}, nil).Once()
		defer runner.AssertExpectations(t)

		followers := logs.NewFollowers(&replyer, discardLogger())

		useCase := NewUseCase(&runner, followers, accepts(), &replyer, gateway.NewStreams(), discardLogger())
		require.NoError(t, useCase.Handle(context.Background(), request(t, written)))

		logged, err := json.Marshal(taskEvents.TaskLogged{
			UUID: taskUUID,
			Lines: []taskEvents.LogLine{
				{Content: "already read", At: written},
				{Content: "new", At: written.Add(time.Second)},
			},
		})
		require.NoError(t, err)
		require.NoError(t, followers.Lines(context.Background(), logged))

		assert.Equal(t, []string{"new"}, said(t, &replyer))
	})

	t.Run("a client that walks away has its stream closed", func(t *testing.T) {
		t.Parallel()

		var (
			runner  runnerMock.MockClient
			replyer messagingMock.RecordingReplyer
		)

		runner.On("TaskLogs", mock.Anything, taskUUID, time.Time{}, backlog).Return([]task.Log{}, nil).Once()

		followers := logs.NewFollowers(&replyer, discardLogger())
		streams := gateway.NewStreams()

		useCase := NewUseCase(&runner, followers, accepts(), &replyer, streams, discardLogger())
		require.NoError(t, useCase.Handle(context.Background(), request(t, time.Time{})))

		require.Equal(t, 1, followers.Len())

		cancellation, err := json.Marshal(&gateway.StreamCancelled{RequestID: requestID})
		require.NoError(t, err)
		require.NoError(t, streams.Handle(context.Background(), cancellation))

		assert.Zero(t, followers.Len(), "nothing is left following a task nobody is watching")
	})

	t.Run("a task that does not exist has no log", func(t *testing.T) {
		t.Parallel()

		var (
			runner  runnerMock.MockClient
			replyer messagingMock.RecordingReplyer
		)

		runner.On("TaskLogs", mock.Anything, taskUUID, time.Time{}, backlog).
			Return([]task.Log(nil), domain.ErrNotExists).Once()

		followers := logs.NewFollowers(&replyer, discardLogger())

		useCase := NewUseCase(&runner, followers, accepts(), &replyer, gateway.NewStreams(), discardLogger())
		require.NoError(t, useCase.Handle(context.Background(), request(t, time.Time{})))

		replies := replyer.Replies()
		require.Len(t, replies, 1)
		assert.Equal(t, domain.ReplyEOF, replies[0].Kind)
		assert.Zero(t, followers.Len(), "nothing follows a task that is not there")
	})

	t.Run("a malformed request is dropped rather than redelivered", func(t *testing.T) {
		t.Parallel()

		var (
			runner  runnerMock.MockClient
			replyer messagingMock.RecordingReplyer
		)

		useCase := NewUseCase(&runner, logs.NewFollowers(&replyer, discardLogger()), accepts(), &replyer, gateway.NewStreams(), discardLogger())

		assert.NoError(t, useCase.Handle(context.Background(), []byte("{")))
		assert.Empty(t, replyer.Replies())
	})
}
