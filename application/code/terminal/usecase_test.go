package terminal

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
	runnerTerminal "github.com/khanzadimahdi/testproject/application/runner/terminal"
	"github.com/khanzadimahdi/testproject/domain"
	"github.com/khanzadimahdi/testproject/domain/runner/task"
	messagingMock "github.com/khanzadimahdi/testproject/infrastructure/messaging/mock"
	runnerMock "github.com/khanzadimahdi/testproject/infrastructure/repository/mocks/runner/manager"
	"github.com/khanzadimahdi/testproject/infrastructure/validator"
	"github.com/khanzadimahdi/testproject/infrastructure/websocket/gateway"
)

func discardLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

func accepts() *validator.MockValidator {
	v := &validator.MockValidator{}
	v.On("Validate", mock.Anything).Return(domain.ValidationErrors{})

	return v
}

const containerUUID = "container-uuid"

func request(t *testing.T) []byte {
	t.Helper()

	payload, err := json.Marshal(Request{ID: "request-id", ContainerUUID: containerUUID})
	require.NoError(t, err)

	return payload
}

// refusal is what the client was told about a terminal it does not get.
func refusal(t *testing.T, replies []domain.Reply) map[string]string {
	t.Helper()

	require.Len(t, replies, 1)
	require.Equal(t, domain.ReplyEOF, replies[0].Kind)

	var body struct {
		Errors map[string]string `json:"errors"`
	}
	require.NoError(t, json.Unmarshal(replies[0].Payload, &body))

	return body.Errors
}

func TestUseCase_Handle(t *testing.T) {
	t.Parallel()

	t.Run("opens a terminal in the container a snippet is running in", func(t *testing.T) {
		t.Parallel()

		var (
			runner   runnerMock.MockClient
			replyer  messagingMock.RecordingReplyer
			attached = runnerMock.NewFakeAttachment()
		)

		runner.On("Container", mock.Anything, containerUUID).Once().
			Return(task.Task{UUID: containerUUID, Kind: task.KindJob, OwnerUUID: runCode.CodeRunnerOwnerUUID}, nil)
		runner.On("AttachContainer", mock.Anything, containerUUID, mock.Anything).Once().Return(attached, nil)
		defer runner.AssertExpectations(t)

		sessions := runnerTerminal.NewSessions(&replyer, gateway.NewStreams(), discardLogger())

		require.NoError(t, NewUseCase(&runner, accepts(), sessions, discardLogger()).
			Handle(context.Background(), request(t)))
	})

	t.Run("a container the code runner does not own is not there", func(t *testing.T) {
		t.Parallel()

		var (
			runner  runnerMock.MockClient
			replyer messagingMock.RecordingReplyer
		)

		// somebody's own container from the dashboard: naming it here does not
		// make it a snippet's.
		runner.On("Container", mock.Anything, containerUUID).Once().
			Return(task.Task{UUID: containerUUID, Kind: task.KindService, OwnerUUID: "somebody"}, nil)
		defer runner.AssertExpectations(t)

		sessions := runnerTerminal.NewSessions(&replyer, gateway.NewStreams(), discardLogger())

		require.NoError(t, NewUseCase(&runner, accepts(), sessions, discardLogger()).
			Handle(context.Background(), request(t)))

		assert.Equal(t, "not_exists", refusal(t, replyer.Replies())["container_uuid"])
		runner.AssertNotCalled(t, "AttachContainer", mock.Anything, mock.Anything, mock.Anything)
	})

	t.Run("a job that belongs to somebody is not a snippet's either", func(t *testing.T) {
		t.Parallel()

		var (
			runner  runnerMock.MockClient
			replyer messagingMock.RecordingReplyer
		)

		runner.On("Container", mock.Anything, containerUUID).Once().
			Return(task.Task{UUID: containerUUID, Kind: task.KindJob, OwnerUUID: "somebody"}, nil)
		defer runner.AssertExpectations(t)

		sessions := runnerTerminal.NewSessions(&replyer, gateway.NewStreams(), discardLogger())

		require.NoError(t, NewUseCase(&runner, accepts(), sessions, discardLogger()).
			Handle(context.Background(), request(t)))

		assert.Equal(t, "not_exists", refusal(t, replyer.Replies())["container_uuid"])
		runner.AssertNotCalled(t, "AttachContainer", mock.Anything, mock.Anything, mock.Anything)
	})
}
