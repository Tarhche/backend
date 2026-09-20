package describeauthorization

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/khanzadimahdi/testproject/application/oauth"
	"github.com/khanzadimahdi/testproject/domain"
	"github.com/khanzadimahdi/testproject/domain/oauth/client"
	"github.com/khanzadimahdi/testproject/infrastructure/crypto/ecdsa"
	"github.com/khanzadimahdi/testproject/infrastructure/jwt"
	clients "github.com/khanzadimahdi/testproject/infrastructure/repository/mocks/oauth/clients"
)

func pending(t *testing.T) (*oauth.Requests, string) {
	t.Helper()

	privateKey, err := ecdsa.Generate()
	require.NoError(t, err)

	requests := oauth.NewRequests(jwt.NewJWT(privateKey, privateKey.Public()))

	token, err := requests.Generate(context.Background(), oauth.AuthorizationRequest{
		ClientID:    "client-id",
		ClientName:  "what the request says it is called",
		RedirectURI: "https://agent.example/callback",
		Scope:       oauth.Scope,
	})
	require.NoError(t, err)

	return requests, token
}

func TestUseCase_Execute(t *testing.T) {
	t.Parallel()

	t.Run("what is shown is what the application is registered as, not what it says", func(t *testing.T) {
		t.Parallel()

		requests, token := pending(t)

		var repository clients.MockClientsRepository
		repository.On("GetOne", mock.Anything, "client-id").Return(client.Client{
			ID:   "client-id",
			Name: "An agent",
			URI:  "https://agent.example",
		}, nil).Once()
		defer repository.AssertExpectations(t)

		response, err := NewUseCase(&repository, requests).Execute(context.Background(), &Request{RequestToken: token})
		require.NoError(t, err)

		assert.Equal(t, "An agent", response.ClientName)
		assert.Equal(t, "https://agent.example", response.ClientURI)
		assert.Equal(t, "https://agent.example/callback", response.RedirectURI)
		assert.Equal(t, []string{oauth.Scope}, response.Scopes)
	})

	t.Run("an application that is no longer registered is not vouched for", func(t *testing.T) {
		t.Parallel()

		requests, token := pending(t)

		var repository clients.MockClientsRepository
		repository.On("GetOne", mock.Anything, "client-id").Return(client.Client{}, domain.ErrNotExists).Once()
		defer repository.AssertExpectations(t)

		_, err := NewUseCase(&repository, requests).Execute(context.Background(), &Request{RequestToken: token})

		var oauthError *oauth.Error
		require.ErrorAs(t, err, &oauthError)
		assert.Equal(t, oauth.ErrorInvalidClient, oauthError.Code)
	})

	t.Run("a request nobody here signed says nothing", func(t *testing.T) {
		t.Parallel()

		requests, _ := pending(t)

		var repository clients.MockClientsRepository
		defer repository.AssertExpectations(t)

		_, err := NewUseCase(&repository, requests).Execute(context.Background(), &Request{RequestToken: "not-a-token"})

		var oauthError *oauth.Error
		require.ErrorAs(t, err, &oauthError)
		assert.Equal(t, oauth.ErrorInvalidRequest, oauthError.Code)
	})
}
