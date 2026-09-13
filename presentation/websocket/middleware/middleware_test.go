package middleware

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/khanzadimahdi/testproject/application/auth"
	"github.com/khanzadimahdi/testproject/domain"
	"github.com/khanzadimahdi/testproject/domain/permission"
	"github.com/khanzadimahdi/testproject/domain/user"
	"github.com/khanzadimahdi/testproject/infrastructure/crypto/ecdsa"
	"github.com/khanzadimahdi/testproject/infrastructure/jwt"
	messagingMock "github.com/khanzadimahdi/testproject/infrastructure/messaging/mock"
	"github.com/khanzadimahdi/testproject/infrastructure/repository/mocks/users"
)

const (
	userUUID  = "user-uuid"
	requestID = "server-side-request-id"
)

// handled records what reached the use case behind the middleware.
type handled struct {
	calls int
	asked string
}

func (h *handled) handler() domain.MessageHandler {
	return domain.MessageHandlerFunc(func(ctx context.Context, _ []byte) error {
		h.calls++
		h.asked = auth.UUIDFromContext(ctx)

		return nil
	})
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

func payload(t *testing.T, token string) []byte {
	t.Helper()

	data, err := json.Marshal(map[string]any{
		"id":           requestID,
		"access_token": token,
		"task_uuid":    "task-uuid",
	})
	require.NoError(t, err)

	return data
}

// refusal is the single reply a refused request is answered with.
func refusal(t *testing.T, replyer *messagingMock.RecordingReplyer) map[string]string {
	t.Helper()

	replies := replyer.Replies()
	require.Len(t, replies, 1)
	require.Equal(t, domain.ReplyEOF, replies[0].Kind)
	require.Equal(t, requestID, replies[0].RequestID)

	var answer struct {
		Errors map[string]string `json:"errors"`
	}
	require.NoError(t, json.Unmarshal(replies[0].Payload, &answer))

	return answer.Errors
}

func TestAuthenticate_Handle(t *testing.T) {
	t.Parallel()

	t.Run("a request carries who it is from to the use case", func(t *testing.T) {
		t.Parallel()

		authenticator, token := signedIn(t)

		var (
			next    handled
			replyer messagingMock.RecordingReplyer
		)

		middleware := NewAuthenticateMiddleware(next.handler(), authenticator, &replyer)

		require.NoError(t, middleware.Handle(context.Background(), payload(t, token)))

		assert.Equal(t, 1, next.calls)
		assert.Equal(t, userUUID, next.asked, "the use case is told who is asking")
		assert.Empty(t, replyer.Replies())
	})

	t.Run("a token that identifies nobody opens nothing", func(t *testing.T) {
		t.Parallel()

		authenticator, _ := signedIn(t)

		var (
			next    handled
			replyer messagingMock.RecordingReplyer
		)

		middleware := NewAuthenticateMiddleware(next.handler(), authenticator, &replyer)

		require.NoError(t, middleware.Handle(context.Background(), payload(t, "not-a-token")))

		assert.Zero(t, next.calls)
		assert.Equal(t, map[string]string{"access_token": "unauthenticated"}, refusal(t, &replyer))
	})

	t.Run("a malformed request is dropped rather than redelivered", func(t *testing.T) {
		t.Parallel()

		authenticator, _ := signedIn(t)

		var (
			next    handled
			replyer messagingMock.RecordingReplyer
		)

		middleware := NewAuthenticateMiddleware(next.handler(), authenticator, &replyer)

		assert.NoError(t, middleware.Handle(context.Background(), []byte("{")))
		assert.Zero(t, next.calls)
		assert.Empty(t, replyer.Replies())
	})
}

func TestAuthorize_Handle(t *testing.T) {
	t.Parallel()

	asking := func() context.Context {
		return auth.ToContext(context.Background(), &user.User{UUID: userUUID})
	}

	t.Run("a request from somebody who holds the permission reaches the use case", func(t *testing.T) {
		t.Parallel()

		var (
			next       handled
			authorizer domain.MockAuthorizer
			replyer    messagingMock.RecordingReplyer
		)

		authorizer.On("Authorize", mock.Anything, userUUID, permission.RunnerTasksLogs).Return(true, nil).Once()
		defer authorizer.AssertExpectations(t)

		middleware := NewAuthorizeMiddleware(next.handler(), &authorizer, permission.RunnerTasksLogs, &replyer)

		require.NoError(t, middleware.Handle(asking(), payload(t, "")))

		assert.Equal(t, 1, next.calls)
		assert.Empty(t, replyer.Replies())
	})

	t.Run("a request from somebody who does not is refused", func(t *testing.T) {
		t.Parallel()

		var (
			next       handled
			authorizer domain.MockAuthorizer
			replyer    messagingMock.RecordingReplyer
		)

		authorizer.On("Authorize", mock.Anything, userUUID, permission.RunnerTasksLogs).Return(false, nil).Once()

		middleware := NewAuthorizeMiddleware(next.handler(), &authorizer, permission.RunnerTasksLogs, &replyer)

		require.NoError(t, middleware.Handle(asking(), payload(t, "")))

		assert.Zero(t, next.calls)
		assert.Equal(t, map[string]string{"access_token": "forbidden"}, refusal(t, &replyer))
	})

	t.Run("a permission that cannot be read is the message's failure, not the client's", func(t *testing.T) {
		t.Parallel()

		var (
			next       handled
			authorizer domain.MockAuthorizer
			replyer    messagingMock.RecordingReplyer
		)

		unreadable := errors.New("the roles could not be read")
		authorizer.On("Authorize", mock.Anything, userUUID, permission.RunnerTasksLogs).Return(false, unreadable).Once()

		middleware := NewAuthorizeMiddleware(next.handler(), &authorizer, permission.RunnerTasksLogs, &replyer)

		// returned rather than replied: the message is redelivered instead of
		// telling a client it may not do what it may.
		assert.ErrorIs(t, middleware.Handle(asking(), payload(t, "")), unreadable)
		assert.Zero(t, next.calls)
		assert.Empty(t, replyer.Replies())
	})
}
