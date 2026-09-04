package followContainerLogs

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

	"github.com/khanzadimahdi/testproject/application/access"
	"github.com/khanzadimahdi/testproject/application/auth"
	runnerAccess "github.com/khanzadimahdi/testproject/application/dashboard/runner/access"
	"github.com/khanzadimahdi/testproject/domain"
	"github.com/khanzadimahdi/testproject/domain/permission"
	"github.com/khanzadimahdi/testproject/domain/runner/container"
	"github.com/khanzadimahdi/testproject/domain/runner/task"
	"github.com/khanzadimahdi/testproject/domain/user"
	"github.com/khanzadimahdi/testproject/infrastructure/crypto/ecdsa"
	"github.com/khanzadimahdi/testproject/infrastructure/jwt"
	messagingMock "github.com/khanzadimahdi/testproject/infrastructure/messaging/mock"
	runnerMock "github.com/khanzadimahdi/testproject/infrastructure/repository/mocks/runner/manager"
	"github.com/khanzadimahdi/testproject/infrastructure/repository/mocks/users"
	"github.com/khanzadimahdi/testproject/infrastructure/validator"
	"github.com/khanzadimahdi/testproject/infrastructure/websocket/gateway"
)

const (
	containerUUID = "container-uuid"
	userUUID      = "user-uuid"
	requestID     = "server-side-request-id"
)

func discardLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

func accepts() *validator.MockValidator {
	v := &validator.MockValidator{}
	v.On("Validate", mock.Anything).Return(domain.ValidationErrors{})

	return v
}

func signedIn(t *testing.T) (*auth.Authenticator, string) {
	t.Helper()

	privateKey, err := ecdsa.Generate()
	require.NoError(t, err)

	j := jwt.NewJWT(privateKey, privateKey.Public())

	claims := jwt.NewClaimsBuilder()
	claims.SetSubject(userUUID)
	claims.SetAudience([]string{auth.AccessToken})
	claims.SetExpirationTime(time.Now().Add(time.Minute))

	token, err := j.Generate(context.Background(), claims.Build())
	require.NoError(t, err)

	var userRepository users.MockUsersRepository
	userRepository.On("GetOne", mock.Anything, userUUID).Return(user.User{UUID: userUUID}, nil).Maybe()

	return auth.NewAuthenticator(j, &userRepository), token
}

func request(t *testing.T, token string, after time.Time) []byte {
	t.Helper()

	payload, err := json.Marshal(Request{
		ID:            requestID,
		ContainerUUID: containerUUID,
		AccessToken:   token,
		After:         after,
	})
	require.NoError(t, err)

	return payload
}

// mayTouchAnything is a guard that lets whoever is asking act on whatever they
// named: which of the two permissions they hold, and whose container it is, is
// covered where the guard is.
func mayTouchAnything(runner *runnerMock.MockClient) *runnerAccess.Guard {
	// the guard reads the container before it answers; whose it is does not
	// matter to somebody who may touch them all.
	runner.On("Container", mock.Anything, mock.Anything).Return(task.Task{UUID: containerUUID}, nil).Maybe()

	authorizer := &domain.MockAuthorizer{}
	authorizer.On("Authorize", mock.Anything, mock.Anything, mock.Anything).Return(true, nil).Maybe()

	return runnerAccess.NewGuard(runner, access.NewGuard(authorizer))
}

// mayTouchNothing is a guard that refuses whoever is asking, whatever they
// named.
func mayTouchNothing(runner *runnerMock.MockClient) *runnerAccess.Guard {
	authorizer := &domain.MockAuthorizer{}
	authorizer.On("Authorize", mock.Anything, mock.Anything, mock.Anything).Return(false, nil).Maybe()

	return runnerAccess.NewGuard(runner, access.NewGuard(authorizer))
}

func TestUseCase_Handle(t *testing.T) {
	t.Parallel()

	t.Run("streams a container's lines back as they are written", func(t *testing.T) {
		t.Parallel()

		authenticator, token := signedIn(t)

		var (
			runner     runnerMock.MockClient
			authorizer domain.MockAuthorizer
			replyer    messagingMock.RecordingReplyer
		)

		stream := runnerMock.NewFakeLogStream()
		written := time.Date(2026, 9, 3, 9, 0, 0, 0, time.UTC)

		authorizer.On("Authorize", mock.Anything, userUUID, permission.RunnerContainersLogs).Return(true, nil)
		runner.On("FollowContainerLogs", mock.Anything, containerUUID, time.Time{}).Return(stream, nil).Once()
		defer runner.AssertExpectations(t)

		useCase := NewUseCase(&runner, authenticator, &authorizer, mayTouchAnything(&runner), accepts(), &replyer, gateway.NewStreams(), discardLogger())

		require.NoError(t, useCase.Handle(context.Background(), request(t, token, time.Time{})))

		stream.Emit(container.Log{LogLine: container.LogLine{
			Stream: container.StreamStdout, Content: "listening on :80", At: written,
		}})
		stream.Emit(container.Log{LogLine: container.LogLine{
			Stream: container.StreamStderr, Content: "a warning", At: written.Add(time.Second),
		}})

		require.Eventually(t, func() bool { return len(replyer.Chunks()) == 2 }, 2*time.Second, 10*time.Millisecond)

		var first, second LineResponse
		require.NoError(t, json.Unmarshal(replyer.Chunks()[0], &first))
		require.NoError(t, json.Unmarshal(replyer.Chunks()[1], &second))

		assert.Equal(t, "listening on :80", first.Content)
		assert.Equal(t, "stdout", first.Stream)
		assert.Equal(t, written, first.At)

		assert.Equal(t, "stderr", second.Stream, "which stream a line came from survives the journey")
	})

	t.Run("picks up from where the reader had already caught up to", func(t *testing.T) {
		t.Parallel()

		authenticator, token := signedIn(t)

		var (
			runner     runnerMock.MockClient
			authorizer domain.MockAuthorizer
			replyer    messagingMock.RecordingReplyer
		)

		// the page is rendered with what a container has already written, so
		// the stream starts after the last of those and nothing is shown twice.
		caughtUpTo := time.Date(2026, 9, 3, 9, 0, 0, 0, time.UTC)

		authorizer.On("Authorize", mock.Anything, userUUID, permission.RunnerContainersLogs).Return(true, nil)
		runner.On("FollowContainerLogs", mock.Anything, containerUUID, caughtUpTo).
			Return(runnerMock.NewFakeLogStream(), nil).Once()
		defer runner.AssertExpectations(t)

		useCase := NewUseCase(&runner, authenticator, &authorizer, mayTouchAnything(&runner), accepts(), &replyer, gateway.NewStreams(), discardLogger())

		require.NoError(t, useCase.Handle(context.Background(), request(t, token, caughtUpTo)))
	})

	t.Run("ends the stream when the container stops writing", func(t *testing.T) {
		t.Parallel()

		authenticator, token := signedIn(t)

		var (
			runner     runnerMock.MockClient
			authorizer domain.MockAuthorizer
			replyer    messagingMock.RecordingReplyer
		)

		stream := runnerMock.NewFakeLogStream()

		authorizer.On("Authorize", mock.Anything, userUUID, permission.RunnerContainersLogs).Return(true, nil)
		runner.On("FollowContainerLogs", mock.Anything, containerUUID, time.Time{}).Return(stream, nil).Once()

		useCase := NewUseCase(&runner, authenticator, &authorizer, mayTouchAnything(&runner), accepts(), &replyer, gateway.NewStreams(), discardLogger())
		require.NoError(t, useCase.Handle(context.Background(), request(t, token, time.Time{})))

		require.NoError(t, stream.Close())

		assert.Eventually(t, replyer.Ended, 2*time.Second, 10*time.Millisecond)
	})

	t.Run("a client that walks away has its stream closed", func(t *testing.T) {
		t.Parallel()

		authenticator, token := signedIn(t)

		var (
			runner     runnerMock.MockClient
			authorizer domain.MockAuthorizer
			replyer    messagingMock.RecordingReplyer
		)

		stream := runnerMock.NewFakeLogStream()
		streams := gateway.NewStreams()

		authorizer.On("Authorize", mock.Anything, userUUID, permission.RunnerContainersLogs).Return(true, nil)
		runner.On("FollowContainerLogs", mock.Anything, containerUUID, time.Time{}).Return(stream, nil).Once()

		useCase := NewUseCase(&runner, authenticator, &authorizer, mayTouchAnything(&runner), accepts(), &replyer, streams, discardLogger())
		require.NoError(t, useCase.Handle(context.Background(), request(t, token, time.Time{})))

		require.Eventually(t, func() bool { return streams.Len() == 1 }, 2*time.Second, 10*time.Millisecond)

		cancellation, err := json.Marshal(&gateway.StreamCancelled{RequestID: requestID})
		require.NoError(t, err)
		require.NoError(t, streams.Handle(context.Background(), cancellation))

		assert.Eventually(t, stream.IsClosed, 2*time.Second, 10*time.Millisecond,
			"nothing is left following a container nobody is watching")
	})

	t.Run("a log is refused to somebody who may not read one", func(t *testing.T) {
		t.Parallel()

		authenticator, token := signedIn(t)

		var (
			runner     runnerMock.MockClient
			authorizer domain.MockAuthorizer
			replyer    messagingMock.RecordingReplyer
		)

		runner.On("Container", mock.Anything, containerUUID).Return(task.Task{UUID: containerUUID, OwnerUUID: "somebody-else"}, nil).Maybe()

		useCase := NewUseCase(&runner, authenticator, &authorizer, mayTouchNothing(&runner), accepts(), &replyer, gateway.NewStreams(), discardLogger())

		require.NoError(t, useCase.Handle(context.Background(), request(t, token, time.Time{})))

		replies := replyer.Replies()
		require.Len(t, replies, 1)
		assert.Equal(t, domain.ReplyEOF, replies[0].Kind)

		runner.AssertNotCalled(t, "FollowContainerLogs", mock.Anything, mock.Anything, mock.Anything)
	})

	t.Run("a token that identifies nobody opens nothing", func(t *testing.T) {
		t.Parallel()

		authenticator, _ := signedIn(t)

		var (
			runner     runnerMock.MockClient
			authorizer domain.MockAuthorizer
			replyer    messagingMock.RecordingReplyer
		)

		useCase := NewUseCase(&runner, authenticator, &authorizer, mayTouchAnything(&runner), accepts(), &replyer, gateway.NewStreams(), discardLogger())

		require.NoError(t, useCase.Handle(context.Background(), request(t, "not-a-token", time.Time{})))

		assert.True(t, replyer.Ended())
		runner.AssertNotCalled(t, "FollowContainerLogs", mock.Anything, mock.Anything, mock.Anything)
	})

	t.Run("a malformed request is dropped rather than redelivered", func(t *testing.T) {
		t.Parallel()

		authenticator, _ := signedIn(t)

		var (
			runner     runnerMock.MockClient
			authorizer domain.MockAuthorizer
			replyer    messagingMock.RecordingReplyer
		)

		useCase := NewUseCase(&runner, authenticator, &authorizer, mayTouchAnything(&runner), accepts(), &replyer, gateway.NewStreams(), discardLogger())

		assert.NoError(t, useCase.Handle(context.Background(), []byte("{")))
		assert.Empty(t, replyer.Replies())
	})
}
