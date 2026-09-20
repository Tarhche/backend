package registerclient

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/khanzadimahdi/testproject/application/oauth"
	"github.com/khanzadimahdi/testproject/domain/oauth/client"
	"github.com/khanzadimahdi/testproject/domain/password"
	"github.com/khanzadimahdi/testproject/infrastructure/crypto/argon2"
	clients "github.com/khanzadimahdi/testproject/infrastructure/repository/mocks/oauth/clients"
)

func hasher() password.Hasher {
	return argon2.NewArgon2id(1, 8*1024, 1, 32)
}

func TestUseCase_Execute(t *testing.T) {
	t.Parallel()

	t.Run("an application registers with what it left out filled in", func(t *testing.T) {
		t.Parallel()

		var repository clients.MockClientsRepository
		repository.On("Save", mock.Anything, mock.Anything).Return("client-id", nil).Once()
		defer repository.AssertExpectations(t)

		response, err := NewUseCase(&repository, hasher()).Execute(context.Background(), &Request{
			ClientName:   "An agent",
			RedirectURIs: []string{"https://agent.example/callback"},
		})

		require.NoError(t, err)
		assert.Equal(t, "client-id", response.ClientID)
		assert.Empty(t, response.ClientSecret)
		assert.Equal(t, client.AuthMethodNone, response.TokenEndpointAuthMethod)
		assert.ElementsMatch(t, []string{client.GrantAuthorizationCode, client.GrantRefreshToken}, response.GrantTypes)
		assert.Equal(t, []string{client.ResponseTypeCode}, response.ResponseTypes)
		assert.Equal(t, oauth.Scope, response.Scope)
	})

	t.Run("a registration nobody approves does not last", func(t *testing.T) {
		t.Parallel()

		var saved *client.Client

		var repository clients.MockClientsRepository
		repository.On("Save", mock.Anything, mock.Anything).Run(func(args mock.Arguments) {
			saved = args.Get(1).(*client.Client)
		}).Return("client-id", nil).Once()
		defer repository.AssertExpectations(t)

		_, err := NewUseCase(&repository, hasher()).Execute(context.Background(), &Request{
			ClientName:   "An agent",
			RedirectURIs: []string{"https://agent.example/callback"},
		})
		require.NoError(t, err)

		// anybody may register, so a registration is kept on sufferance until
		// somebody approves it
		require.NotNil(t, saved)
		assert.False(t, saved.ExpiredAt.IsZero())
		assert.Equal(t, client.UnapprovedLifetime, saved.ExpiredAt.Sub(saved.CreatedAt))
	})

	t.Run("an application that keeps a secret is given one, once", func(t *testing.T) {
		t.Parallel()

		var saved *client.Client

		var repository clients.MockClientsRepository
		repository.On("Save", mock.Anything, mock.Anything).Run(func(args mock.Arguments) {
			saved = args.Get(1).(*client.Client)
		}).Return("client-id", nil).Once()
		defer repository.AssertExpectations(t)

		h := hasher()
		response, err := NewUseCase(&repository, h).Execute(context.Background(), &Request{
			ClientName:              "A service",
			RedirectURIs:            []string{"https://service.example/callback"},
			TokenEndpointAuthMethod: client.AuthMethodSecretPost,
		})

		require.NoError(t, err)
		require.NotEmpty(t, response.ClientSecret)
		require.NotNil(t, response.ClientSecretExpiresAt)
		assert.Zero(t, *response.ClientSecretExpiresAt)

		// what is kept is not the secret, and answers to it
		require.NotNil(t, saved)
		assert.NotContains(t, string(saved.Secret.Value), response.ClientSecret)
		assert.True(t, h.Equal(context.Background(), []byte(response.ClientSecret), saved.Secret.Value, saved.Secret.Salt))
	})

	t.Run("an address a session must not be sent to is refused", func(t *testing.T) {
		t.Parallel()

		for name, uri := range map[string]string{
			"plain http somewhere else": "http://agent.example/callback",
			"not absolute":              "/callback",
			"carrying a fragment":       "https://agent.example/callback#code",
		} {
			t.Run(name, func(t *testing.T) {
				var repository clients.MockClientsRepository
				defer repository.AssertExpectations(t)

				_, err := NewUseCase(&repository, hasher()).Execute(context.Background(), &Request{
					ClientName:   "An agent",
					RedirectURIs: []string{uri},
				})

				var oauthError *oauth.Error
				require.ErrorAs(t, err, &oauthError)
				assert.Equal(t, oauth.ErrorInvalidRedirectURI, oauthError.Code)
			})
		}
	})

	t.Run("an application that runs on somebody's own machine is answered where it can be", func(t *testing.T) {
		t.Parallel()

		for name, uri := range map[string]string{
			"the loopback interface": "http://127.0.0.1:41293/callback",
			"localhost":              "http://localhost:6274/oauth/callback",
			"a scheme of its own":    "claude://oauth/callback",
		} {
			t.Run(name, func(t *testing.T) {
				var repository clients.MockClientsRepository
				repository.On("Save", mock.Anything, mock.Anything).Return("client-id", nil).Once()
				defer repository.AssertExpectations(t)

				_, err := NewUseCase(&repository, hasher()).Execute(context.Background(), &Request{
					ClientName:   "An agent",
					RedirectURIs: []string{uri},
				})

				require.NoError(t, err)
			})
		}
	})

	t.Run("what cannot be honoured is refused", func(t *testing.T) {
		t.Parallel()

		for name, request := range map[string]*Request{
			"no address at all":                 {ClientName: "An agent"},
			"a grant we do not do":              {RedirectURIs: []string{"https://agent.example/c"}, GrantTypes: []string{"password"}},
			"only refreshing":                   {RedirectURIs: []string{"https://agent.example/c"}, GrantTypes: []string{client.GrantRefreshToken}},
			"a response we do not":              {RedirectURIs: []string{"https://agent.example/c"}, ResponseTypes: []string{"token"}},
			"a way of proving itself we do not": {RedirectURIs: []string{"https://agent.example/c"}, TokenEndpointAuthMethod: "private_key_jwt"},
			"a scope we do not offer":           {RedirectURIs: []string{"https://agent.example/c"}, Scope: "everything"},
		} {
			t.Run(name, func(t *testing.T) {
				var repository clients.MockClientsRepository
				defer repository.AssertExpectations(t)

				_, err := NewUseCase(&repository, hasher()).Execute(context.Background(), request)

				var oauthError *oauth.Error
				require.ErrorAs(t, err, &oauthError)
			})
		}
	})
}
