package approveauthorization

import (
	"context"
	"net/url"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/khanzadimahdi/testproject/application/oauth"
	"github.com/khanzadimahdi/testproject/domain/oauth/client"
	"github.com/khanzadimahdi/testproject/domain/oauth/grant"
	"github.com/khanzadimahdi/testproject/domain/password"
	"github.com/khanzadimahdi/testproject/infrastructure/crypto/argon2"
	"github.com/khanzadimahdi/testproject/infrastructure/crypto/ecdsa"
	"github.com/khanzadimahdi/testproject/infrastructure/jwt"
	clients "github.com/khanzadimahdi/testproject/infrastructure/repository/mocks/oauth/clients"
	grants "github.com/khanzadimahdi/testproject/infrastructure/repository/mocks/oauth/grants"
)

const userUUID = "user-uuid"

func hasher() password.Hasher {
	return argon2.NewArgon2id(1, 8*1024, 1, 32)
}

func registered() client.Client {
	return client.Client{
		ID:           "client-id",
		Name:         "An agent",
		RedirectURIs: []string{"https://agent.example/callback"},
		GrantTypes:   []string{client.GrantAuthorizationCode},
	}
}

// pending is a request that reached the page, and the thing that reads it back.
func pending(t *testing.T) (*oauth.Requests, string) {
	t.Helper()

	privateKey, err := ecdsa.Generate()
	require.NoError(t, err)

	requests := oauth.NewRequests(jwt.NewJWT(privateKey, privateKey.Public()))

	token, err := requests.Generate(context.Background(), oauth.AuthorizationRequest{
		ClientID:            "client-id",
		ClientName:          "An agent",
		RedirectURI:         "https://agent.example/callback",
		State:               "state-from-the-client",
		Scope:               oauth.Scope,
		CodeChallenge:       "a-challenge",
		CodeChallengeMethod: grant.ChallengeMethodS256,
	})
	require.NoError(t, err)

	return requests, token
}

func TestUseCase_Execute(t *testing.T) {
	t.Parallel()

	t.Run("an approval is written down and the application is sent the code", func(t *testing.T) {
		t.Parallel()

		requests, token := pending(t)

		var saved *grant.Grant

		var clientRepository clients.MockClientsRepository
		clientRepository.On("GetOne", mock.Anything, "client-id").Return(registered(), nil).Once()
		defer clientRepository.AssertExpectations(t)

		var grantRepository grants.MockGrantsRepository
		grantRepository.On("Save", mock.Anything, mock.Anything).Run(func(args mock.Arguments) {
			saved = args.Get(1).(*grant.Grant)
			saved.ID = "grant-id"
		}).Return("grant-id", nil).Once()
		defer grantRepository.AssertExpectations(t)

		h := hasher()

		response, err := NewUseCase(&clientRepository, &grantRepository, requests, h).Execute(context.Background(), &Request{
			RequestToken: token,
			Approved:     true,
			UserUUID:     userUUID,
		})
		require.NoError(t, err)

		sent, err := url.Parse(response.RedirectTo)
		require.NoError(t, err)
		assert.Equal(t, "agent.example", sent.Host)
		assert.Equal(t, "state-from-the-client", sent.Query().Get("state"))

		code := sent.Query().Get("code")
		require.NotEmpty(t, code)

		// the code is the grant's name and a secret of its own, and what was
		// kept answers to the secret without being it
		id, secret, found := strings.Cut(code, ".")
		require.True(t, found)
		assert.Equal(t, "grant-id", id)

		require.NotNil(t, saved)
		assert.Equal(t, userUUID, saved.UserUUID)
		assert.Equal(t, "a-challenge", saved.CodeChallenge)
		assert.True(t, h.Equal(context.Background(), []byte(secret), saved.Secret.Value, saved.Secret.Salt))
		assert.False(t, saved.IsExpired())
	})

	t.Run("a refusal is sent back as one, and nothing is written down", func(t *testing.T) {
		t.Parallel()

		requests, token := pending(t)

		var clientRepository clients.MockClientsRepository
		clientRepository.On("GetOne", mock.Anything, "client-id").Return(registered(), nil).Once()
		defer clientRepository.AssertExpectations(t)

		var grantRepository grants.MockGrantsRepository
		defer grantRepository.AssertExpectations(t)

		response, err := NewUseCase(&clientRepository, &grantRepository, requests, hasher()).Execute(context.Background(), &Request{
			RequestToken: token,
			Approved:     false,
			UserUUID:     userUUID,
		})
		require.NoError(t, err)

		sent, err := url.Parse(response.RedirectTo)
		require.NoError(t, err)
		assert.Equal(t, oauth.ErrorAccessDenied, sent.Query().Get("error"))
		assert.Equal(t, "state-from-the-client", sent.Query().Get("state"))
		assert.Empty(t, sent.Query().Get("code"))
	})

	t.Run("a session somebody is standing in cannot be given away", func(t *testing.T) {
		t.Parallel()

		requests, token := pending(t)

		var clientRepository clients.MockClientsRepository
		defer clientRepository.AssertExpectations(t)

		var grantRepository grants.MockGrantsRepository
		defer grantRepository.AssertExpectations(t)

		_, err := NewUseCase(&clientRepository, &grantRepository, requests, hasher()).Execute(context.Background(), &Request{
			RequestToken:     token,
			Approved:         true,
			UserUUID:         userUUID,
			ImpersonatorUUID: "somebody-else",
		})

		var oauthError *oauth.Error
		require.ErrorAs(t, err, &oauthError)
		assert.Equal(t, oauth.ErrorAccessDenied, oauthError.Code)
	})

	t.Run("nobody signed in approves nothing", func(t *testing.T) {
		t.Parallel()

		requests, token := pending(t)

		var clientRepository clients.MockClientsRepository
		defer clientRepository.AssertExpectations(t)

		var grantRepository grants.MockGrantsRepository
		defer grantRepository.AssertExpectations(t)

		_, err := NewUseCase(&clientRepository, &grantRepository, requests, hasher()).Execute(context.Background(), &Request{
			RequestToken: token,
			Approved:     true,
		})

		var oauthError *oauth.Error
		require.ErrorAs(t, err, &oauthError)
		assert.Equal(t, oauth.ErrorAccessDenied, oauthError.Code)
	})

	t.Run("a request nobody here signed is worth nothing", func(t *testing.T) {
		t.Parallel()

		requests, _ := pending(t)
		_, elsewhere := pending(t)

		var clientRepository clients.MockClientsRepository
		defer clientRepository.AssertExpectations(t)

		var grantRepository grants.MockGrantsRepository
		defer grantRepository.AssertExpectations(t)

		_, err := NewUseCase(&clientRepository, &grantRepository, requests, hasher()).Execute(context.Background(), &Request{
			RequestToken: elsewhere,
			Approved:     true,
			UserUUID:     userUUID,
		})

		var oauthError *oauth.Error
		require.ErrorAs(t, err, &oauthError)
		assert.Equal(t, oauth.ErrorInvalidRequest, oauthError.Code)
	})
}
