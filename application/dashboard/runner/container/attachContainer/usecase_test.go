package attachContainer

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
	runnerAccess "github.com/khanzadimahdi/testproject/application/dashboard/runner/access"
	"github.com/khanzadimahdi/testproject/domain"
	"github.com/khanzadimahdi/testproject/domain/permission"
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

// signedIn builds an authenticator that recognises one real token, so the
// permission check under test is reached the way it is in production.
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

func request(t *testing.T, token string) []byte {
	t.Helper()

	payload, err := json.Marshal(Request{
		ID:            requestID,
		ContainerUUID: containerUUID,
		AccessToken:   token,
	})
	require.NoError(t, err)

	return payload
}

// refusal reads back what a stream that never opened told its client.
func refusal(t *testing.T, replies []domain.Reply) domain.ValidationErrors {
	t.Helper()

	require.Len(t, replies, 1)
	assert.Equal(t, domain.ReplyEOF, replies[0].Kind, "a stream that never opened is closed off, not left hanging")

	var body struct {
		Errors domain.ValidationErrors `json:"errors"`
	}
	require.NoError(t, json.Unmarshal(replies[0].Payload, &body))

	return body.Errors
}

// mayTouchAnything reaches whatever was named: somebody who may see every
// container gets the one they asked for.
func mayTouchAnything(runner *runnerMock.MockClient) *runnerAccess.Containers {
	runner.On("Container", mock.Anything, mock.Anything).Return(task.Task{UUID: containerUUID}, nil).Maybe()

	authorizer := &domain.MockAuthorizer{}
	authorizer.On("Authorize", mock.Anything, mock.Anything, mock.Anything).Return(true, nil).Maybe()

	return runnerAccess.NewContainers(runner, authorizer)
}

// mayTouchNothing reaches nothing: to somebody who holds neither permission,
// the container they named is not there.
func mayTouchNothing(runner *runnerMock.MockClient) *runnerAccess.Containers {
	authorizer := &domain.MockAuthorizer{}
	authorizer.On("Authorize", mock.Anything, mock.Anything, mock.Anything).Return(false, nil).Maybe()

	return runnerAccess.NewContainers(runner, authorizer)
}

func TestUseCase_Handle(t *testing.T) {
	t.Parallel()

	t.Run("streams a terminal's output back, chunk by chunk", func(t *testing.T) {
		t.Parallel()

		authenticator, token := signedIn(t)

		var (
			runner     runnerMock.MockClient
			authorizer domain.MockAuthorizer
			replyer    messagingMock.RecordingReplyer
		)

		attachment := runnerMock.NewFakeAttachment()

		authorizer.On("Authorize", mock.Anything, userUUID, permission.RunnerContainersAttach).Return(true, nil)
		runner.On("AttachContainer", mock.Anything, containerUUID, []string(nil)).Return(attachment, nil).Once()
		defer runner.AssertExpectations(t)

		useCase := NewUseCase(&runner, authenticator, &authorizer, mayTouchAnything(&runner), accepts(), &replyer, gateway.NewStreams(), discardLogger())

		require.NoError(t, useCase.Handle(context.Background(), request(t, token)))

		attachment.Emit("/ # ")
		attachment.Emit("hello\n")

		require.Eventually(t, func() bool { return len(replyer.Chunks()) == 2 }, 2*time.Second, 10*time.Millisecond)

		chunks := replyer.Chunks()
		assert.Equal(t, "/ # ", string(chunks[0]))
		assert.Equal(t, "hello\n", string(chunks[1]))

		for _, reply := range replyer.Replies() {
			assert.Equal(t, requestID, reply.RequestID)
		}
	})

	t.Run("ends the stream when the command does", func(t *testing.T) {
		t.Parallel()

		authenticator, token := signedIn(t)

		var (
			runner     runnerMock.MockClient
			authorizer domain.MockAuthorizer
			replyer    messagingMock.RecordingReplyer
		)

		attachment := runnerMock.NewFakeAttachment()

		authorizer.On("Authorize", mock.Anything, userUUID, permission.RunnerContainersAttach).Return(true, nil)
		runner.On("AttachContainer", mock.Anything, containerUUID, []string(nil)).Return(attachment, nil).Once()

		useCase := NewUseCase(&runner, authenticator, &authorizer, mayTouchAnything(&runner), accepts(), &replyer, gateway.NewStreams(), discardLogger())

		require.NoError(t, useCase.Handle(context.Background(), request(t, token)))

		attachment.Emit("bye\n")
		require.Eventually(t, func() bool { return len(replyer.Chunks()) == 1 }, 2*time.Second, 10*time.Millisecond)

		require.NoError(t, attachment.Close())

		assert.Eventually(t, replyer.Ended, 2*time.Second, 10*time.Millisecond)
	})

	t.Run("carries what is typed to the terminal it belongs to", func(t *testing.T) {
		t.Parallel()

		authenticator, token := signedIn(t)

		var (
			runner     runnerMock.MockClient
			authorizer domain.MockAuthorizer
			replyer    messagingMock.RecordingReplyer
		)

		attachment := runnerMock.NewFakeAttachment()

		authorizer.On("Authorize", mock.Anything, userUUID, permission.RunnerContainersAttach).Return(true, nil)
		runner.On("AttachContainer", mock.Anything, containerUUID, []string(nil)).Return(attachment, nil).Once()

		useCase := NewUseCase(&runner, authenticator, &authorizer, mayTouchAnything(&runner), accepts(), &replyer, gateway.NewStreams(), discardLogger())
		require.NoError(t, useCase.Handle(context.Background(), request(t, token)))

		keys, err := json.Marshal(Input{ID: requestID, Data: []byte("ls -la\n")})
		require.NoError(t, err)
		require.NoError(t, useCase.HandleInput(context.Background(), keys))

		resize, err := json.Marshal(Input{ID: requestID, Type: "resize", Rows: 24, Cols: 100})
		require.NoError(t, err)
		require.NoError(t, useCase.HandleInput(context.Background(), resize))

		assert.Equal(t, "ls -la\n", attachment.Typed())
		assert.Equal(t, [][2]uint{{24, 100}}, attachment.Resizes())
	})

	t.Run("input for a terminal this replica does not hold is left alone", func(t *testing.T) {
		t.Parallel()

		authenticator, _ := signedIn(t)

		var (
			runner     runnerMock.MockClient
			authorizer domain.MockAuthorizer
			replyer    messagingMock.RecordingReplyer
		)

		useCase := NewUseCase(&runner, authenticator, &authorizer, mayTouchAnything(&runner), accepts(), &replyer, gateway.NewStreams(), discardLogger())

		keys, err := json.Marshal(Input{ID: "a-terminal-elsewhere", Data: []byte("ls\n")})
		require.NoError(t, err)

		assert.NoError(t, useCase.HandleInput(context.Background(), keys))
	})

	t.Run("a client that walks away has its terminal closed", func(t *testing.T) {
		t.Parallel()

		authenticator, token := signedIn(t)

		var (
			runner     runnerMock.MockClient
			authorizer domain.MockAuthorizer
			replyer    messagingMock.RecordingReplyer
		)

		attachment := runnerMock.NewFakeAttachment()
		streams := gateway.NewStreams()

		authorizer.On("Authorize", mock.Anything, userUUID, permission.RunnerContainersAttach).Return(true, nil)
		runner.On("AttachContainer", mock.Anything, containerUUID, []string(nil)).Return(attachment, nil).Once()

		useCase := NewUseCase(&runner, authenticator, &authorizer, mayTouchAnything(&runner), accepts(), &replyer, streams, discardLogger())
		require.NoError(t, useCase.Handle(context.Background(), request(t, token)))

		require.Eventually(t, func() bool { return streams.Len() == 1 }, 2*time.Second, 10*time.Millisecond)

		// what the gateway announces when a client is gone.
		cancellation, err := json.Marshal(&gateway.StreamCancelled{RequestID: requestID})
		require.NoError(t, err)
		require.NoError(t, streams.Handle(context.Background(), cancellation))

		assert.Eventually(t, attachment.IsClosed, 2*time.Second, 10*time.Millisecond,
			"nothing is left running inside a container nobody is watching")
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

		require.NoError(t, useCase.Handle(context.Background(), request(t, "not-a-token")))

		assert.Equal(t, "unauthenticated", refusal(t, replyer.Replies())["access_token"])
		runner.AssertNotCalled(t, "AttachContainer", mock.Anything, mock.Anything, mock.Anything)
	})

	t.Run("a container somebody may not reach is not there for them", func(t *testing.T) {
		t.Parallel()

		authenticator, token := signedIn(t)

		var (
			runner     runnerMock.MockClient
			authorizer domain.MockAuthorizer
			replyer    messagingMock.RecordingReplyer
		)

		// a shell inside somebody's container is the strongest thing on offer,
		// so it is never implied by any other permission.
		authorizer.On("Authorize", mock.Anything, userUUID, permission.RunnerContainersAttach).Return(false, nil)
		runner.On("Container", mock.Anything, containerUUID).Return(task.Task{UUID: containerUUID, OwnerUUID: "somebody-else"}, nil).Maybe()

		useCase := NewUseCase(&runner, authenticator, &authorizer, mayTouchNothing(&runner), accepts(), &replyer, gateway.NewStreams(), discardLogger())

		require.NoError(t, useCase.Handle(context.Background(), request(t, token)))

		assert.Equal(t, "not_exists", refusal(t, replyer.Replies())["container_uuid"])
		runner.AssertNotCalled(t, "AttachContainer", mock.Anything, mock.Anything, mock.Anything)
	})

	t.Run("a container that is not there is reported rather than waited on", func(t *testing.T) {
		t.Parallel()

		authenticator, token := signedIn(t)

		var (
			runner     runnerMock.MockClient
			authorizer domain.MockAuthorizer
			replyer    messagingMock.RecordingReplyer
		)

		authorizer.On("Authorize", mock.Anything, userUUID, permission.RunnerContainersAttach).Return(true, nil)
		runner.On("AttachContainer", mock.Anything, containerUUID, []string(nil)).
			Return(nil, domain.ErrNotExists).Once()

		useCase := NewUseCase(&runner, authenticator, &authorizer, mayTouchAnything(&runner), accepts(), &replyer, gateway.NewStreams(), discardLogger())

		require.NoError(t, useCase.Handle(context.Background(), request(t, token)))

		assert.Equal(t, "not_exists", refusal(t, replyer.Replies())["container_uuid"])
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
