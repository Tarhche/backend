package authorize

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/khanzadimahdi/testproject/application/oauth"
	"github.com/khanzadimahdi/testproject/domain"
	"github.com/khanzadimahdi/testproject/domain/oauth/client"
	"github.com/khanzadimahdi/testproject/domain/oauth/grant"
	"github.com/khanzadimahdi/testproject/infrastructure/crypto/ecdsa"
	"github.com/khanzadimahdi/testproject/infrastructure/jwt"
	clients "github.com/khanzadimahdi/testproject/infrastructure/repository/mocks/oauth/clients"
)

func requests(t *testing.T) *oauth.Requests {
	t.Helper()

	privateKey, err := ecdsa.Generate()
	require.NoError(t, err)

	return oauth.NewRequests(jwt.NewJWT(privateKey, privateKey.Public()))
}

func registered() client.Client {
	return client.Client{
		ID:                      "client-id",
		Name:                    "An agent",
		RedirectURIs:            []string{"https://agent.example/callback"},
		GrantTypes:              []string{client.GrantAuthorizationCode, client.GrantRefreshToken},
		ResponseTypes:           []string{client.ResponseTypeCode},
		TokenEndpointAuthMethod: client.AuthMethodNone,
	}
}

func request() *Request {
	return &Request{
		ClientID:            "client-id",
		RedirectURI:         "https://agent.example/callback",
		ResponseType:        client.ResponseTypeCode,
		Scope:               oauth.Scope,
		State:               "state-from-the-client",
		CodeChallenge:       "a-challenge",
		CodeChallengeMethod: grant.ChallengeMethodS256,
	}
}

func TestUseCase_Execute(t *testing.T) {
	t.Parallel()

	t.Run("a request worth asking about is carried to the page that asks", func(t *testing.T) {
		t.Parallel()

		var repository clients.MockClientsRepository
		repository.On("GetOne", mock.Anything, "client-id").Return(registered(), nil).Once()
		defer repository.AssertExpectations(t)

		r := requests(t)

		response, err := NewUseCase(&repository, r).Execute(context.Background(), request())
		require.NoError(t, err)
		require.Nil(t, response.Error)
		require.NotEmpty(t, response.RequestToken)

		carried, err := r.Verify(context.Background(), response.RequestToken)
		require.NoError(t, err)
		assert.Equal(t, "client-id", carried.ClientID)
		assert.Equal(t, "An agent", carried.ClientName)
		assert.Equal(t, "https://agent.example/callback", carried.RedirectURI)
		assert.Equal(t, "state-from-the-client", carried.State)
		assert.Equal(t, "a-challenge", carried.CodeChallenge)
		assert.Equal(t, oauth.Scope, carried.Scope)
	})

	t.Run("an address the application did not register is not answered at", func(t *testing.T) {
		t.Parallel()

		var repository clients.MockClientsRepository
		repository.On("GetOne", mock.Anything, "client-id").Return(registered(), nil).Once()
		defer repository.AssertExpectations(t)

		asked := request()
		asked.RedirectURI = "https://somewhere.else/callback"

		_, err := NewUseCase(&repository, requests(t)).Execute(context.Background(), asked)

		var oauthError *oauth.Error
		require.ErrorAs(t, err, &oauthError)
		assert.Equal(t, oauth.ErrorInvalidRedirectURI, oauthError.Code)
	})

	t.Run("an application nobody registered is not answered at all", func(t *testing.T) {
		t.Parallel()

		var repository clients.MockClientsRepository
		repository.On("GetOne", mock.Anything, "client-id").Return(client.Client{}, domain.ErrNotExists).Once()
		defer repository.AssertExpectations(t)

		_, err := NewUseCase(&repository, requests(t)).Execute(context.Background(), request())

		var oauthError *oauth.Error
		require.ErrorAs(t, err, &oauthError)
		assert.Equal(t, oauth.ErrorInvalidClient, oauthError.Code)
	})

	t.Run("one registered address stands in for the one that was left out", func(t *testing.T) {
		t.Parallel()

		var repository clients.MockClientsRepository
		repository.On("GetOne", mock.Anything, "client-id").Return(registered(), nil).Once()
		defer repository.AssertExpectations(t)

		asked := request()
		asked.RedirectURI = ""

		response, err := NewUseCase(&repository, requests(t)).Execute(context.Background(), asked)
		require.NoError(t, err)
		require.Nil(t, response.Error)
		assert.Equal(t, "https://agent.example/callback", response.RedirectURI)
	})

	t.Run("what the application got wrong is sent back to the application", func(t *testing.T) {
		t.Parallel()

		for name, wrong := range map[string]struct {
			change func(*Request)
			code   string
		}{
			"a response type we do not do": {
				change: func(r *Request) { r.ResponseType = "token" },
				code:   oauth.ErrorUnsupportedResponseType,
			},
			"no proof key": {
				change: func(r *Request) { r.CodeChallenge = "" },
				code:   oauth.ErrorInvalidRequest,
			},
			"a proof key we do not trust": {
				change: func(r *Request) { r.CodeChallengeMethod = "plain" },
				code:   oauth.ErrorInvalidRequest,
			},
			"a scope we do not offer": {
				change: func(r *Request) { r.Scope = "everything" },
				code:   oauth.ErrorInvalidScope,
			},
		} {
			t.Run(name, func(t *testing.T) {
				var repository clients.MockClientsRepository
				repository.On("GetOne", mock.Anything, "client-id").Return(registered(), nil).Once()
				defer repository.AssertExpectations(t)

				asked := request()
				wrong.change(asked)

				response, err := NewUseCase(&repository, requests(t)).Execute(context.Background(), asked)
				require.NoError(t, err)
				require.NotNil(t, response.Error)
				assert.Equal(t, wrong.code, response.Error.Code)
				assert.Equal(t, "https://agent.example/callback", response.RedirectURI)
				assert.Equal(t, "state-from-the-client", response.State)
				assert.Empty(t, response.RequestToken)
			})
		}
	})
}
