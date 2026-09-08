package watchStacks

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
	"github.com/khanzadimahdi/testproject/application/dashboard/runner/owners"
	"github.com/khanzadimahdi/testproject/domain"
	"github.com/khanzadimahdi/testproject/domain/permission"
	runnerManager "github.com/khanzadimahdi/testproject/domain/runner/manager"
	"github.com/khanzadimahdi/testproject/domain/runner/port"
	"github.com/khanzadimahdi/testproject/domain/runner/stack"
	"github.com/khanzadimahdi/testproject/domain/runner/task"
	"github.com/khanzadimahdi/testproject/domain/user"
	"github.com/khanzadimahdi/testproject/infrastructure/crypto/ecdsa"
	"github.com/khanzadimahdi/testproject/infrastructure/jwt"
	messagingMock "github.com/khanzadimahdi/testproject/infrastructure/messaging/mock"
	runnerMock "github.com/khanzadimahdi/testproject/infrastructure/repository/mocks/runner/manager"
	usersMock "github.com/khanzadimahdi/testproject/infrastructure/repository/mocks/users"
	"github.com/khanzadimahdi/testproject/infrastructure/validator"
	"github.com/khanzadimahdi/testproject/infrastructure/websocket/gateway"
)

const (
	userUUID      = "user-uuid"
	requestID     = "server-side-request-id"
	ingressDomain = "containers.test"
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

	var userRepository usersMock.MockUsersRepository
	userRepository.On("GetOne", mock.Anything, userUUID).Return(user.User{UUID: userUUID}, nil).Maybe()

	return auth.NewAuthenticator(j, &userRepository), token
}

func request(t *testing.T, token string) []byte {
	t.Helper()

	payload, err := json.Marshal(Request{ID: requestID, AccessToken: token})
	require.NoError(t, err)

	return payload
}

// directory answers about whoever a container belongs to. What is made of that
// answer is covered where the presenting is.
func directory() *owners.Directory {
	users := &usersMock.MockUsersRepository{}
	users.On("GetByUUIDs", mock.Anything, mock.Anything).Return([]user.User{{UUID: "owner-uuid", Name: "Mahdi"}}, nil).Maybe()

	return owners.NewDirectory(users)
}

func TestUseCase_Handle(t *testing.T) {
	t.Parallel()

	t.Run("carries what became of a stack back as it happens", func(t *testing.T) {
		t.Parallel()

		authenticator, token := signedIn(t)

		var (
			runner     runnerMock.MockClient
			authorizer domain.MockAuthorizer
			replyer    messagingMock.RecordingReplyer
		)

		stream := runnerMock.NewFakeStackStream()

		authorizer.On("Authorize", mock.Anything, userUUID, permission.RunnerStacksIndex).Return(true, nil)
		runner.On("WatchStacks", mock.Anything).Return(stream, nil).Once()
		defer runner.AssertExpectations(t)

		useCase := NewUseCase(&runner, authenticator, &authorizer, accepts(), &replyer, gateway.NewStreams(), directory(), ingressDomain, discardLogger())

		require.NoError(t, useCase.Handle(context.Background(), request(t, token)))

		stream.Emit(runnerManager.StackChange{
			UUID: "one",
			Stack: runnerManager.Stack{
				Stack: stack.Stack{UUID: "one", Name: "myapp", Slug: "myapp-abcde"},
				State: task.Running,
				Services: []task.Task{{
					UUID:         "web-uuid",
					Name:         "myapp-web",
					Slug:         "web-abcde",
					CurrentState: task.Running,
					Image:        "nginx:1.27-alpine",
					Endpoints:    []task.Endpoint{{ContainerPort: port.Port(80)}},
				}},
			},
		})
		stream.Emit(runnerManager.StackChange{UUID: "two", Deleted: true})

		require.Eventually(t, func() bool { return len(replyer.Chunks()) == 2 }, 2*time.Second, 10*time.Millisecond)

		var changed, deleted ChangeResponse
		require.NoError(t, json.Unmarshal(replyer.Chunks()[0], &changed))
		require.NoError(t, json.Unmarshal(replyer.Chunks()[1], &deleted))

		assert.Equal(t, kindChanged, changed.Kind)
		require.NotNil(t, changed.Stack)
		assert.Equal(t, "running", changed.Stack.State, "a stack's state is read off its services")
		require.Len(t, changed.Stack.Services, 1)
		assert.Equal(t, "web-abcde."+ingressDomain, changed.Stack.Services[0].Endpoints[0].Host,
			"a service that has come up is shown with the address it answers on")

		assert.Equal(t, kindDeleted, deleted.Kind)
		assert.Equal(t, "two", deleted.UUID)
		assert.Nil(t, deleted.Stack, "there is nothing left to describe about a stack that is gone")
	})

	t.Run("ends the stream when the runner stops reporting", func(t *testing.T) {
		t.Parallel()

		authenticator, token := signedIn(t)

		var (
			runner     runnerMock.MockClient
			authorizer domain.MockAuthorizer
			replyer    messagingMock.RecordingReplyer
		)

		stream := runnerMock.NewFakeStackStream()

		authorizer.On("Authorize", mock.Anything, userUUID, permission.RunnerStacksIndex).Return(true, nil)
		runner.On("WatchStacks", mock.Anything).Return(stream, nil).Once()

		useCase := NewUseCase(&runner, authenticator, &authorizer, accepts(), &replyer, gateway.NewStreams(), directory(), ingressDomain, discardLogger())
		require.NoError(t, useCase.Handle(context.Background(), request(t, token)))

		require.NoError(t, stream.Close())

		assert.Eventually(t, replyer.Ended, 2*time.Second, 10*time.Millisecond)
	})

	t.Run("a client that walks away has its watch closed", func(t *testing.T) {
		t.Parallel()

		authenticator, token := signedIn(t)

		var (
			runner     runnerMock.MockClient
			authorizer domain.MockAuthorizer
			replyer    messagingMock.RecordingReplyer
		)

		stream := runnerMock.NewFakeStackStream()
		streams := gateway.NewStreams()

		authorizer.On("Authorize", mock.Anything, userUUID, permission.RunnerStacksIndex).Return(true, nil)
		runner.On("WatchStacks", mock.Anything).Return(stream, nil).Once()

		useCase := NewUseCase(&runner, authenticator, &authorizer, accepts(), &replyer, streams, directory(), ingressDomain, discardLogger())
		require.NoError(t, useCase.Handle(context.Background(), request(t, token)))

		require.Eventually(t, func() bool { return streams.Len() == 1 }, 2*time.Second, 10*time.Millisecond)

		cancellation, err := json.Marshal(&gateway.StreamCancelled{RequestID: requestID})
		require.NoError(t, err)
		require.NoError(t, streams.Handle(context.Background(), cancellation))

		assert.Eventually(t, stream.IsClosed, 2*time.Second, 10*time.Millisecond,
			"nothing is left watching stacks nobody is looking at")
	})

	t.Run("a watch is refused to somebody who may not list stacks", func(t *testing.T) {
		t.Parallel()

		authenticator, token := signedIn(t)

		var (
			runner     runnerMock.MockClient
			authorizer domain.MockAuthorizer
			replyer    messagingMock.RecordingReplyer
		)

		authorizer.On("Authorize", mock.Anything, userUUID, permission.RunnerStacksIndex).Return(false, nil)
		authorizer.On("Authorize", mock.Anything, userUUID, permission.SelfRunnerStacksIndex).Return(false, nil)

		useCase := NewUseCase(&runner, authenticator, &authorizer, accepts(), &replyer, gateway.NewStreams(), directory(), ingressDomain, discardLogger())

		require.NoError(t, useCase.Handle(context.Background(), request(t, token)))

		replies := replyer.Replies()
		require.Len(t, replies, 1)
		assert.Equal(t, domain.ReplyEOF, replies[0].Kind)

		runner.AssertNotCalled(t, "WatchStacks", mock.Anything)
	})
}
