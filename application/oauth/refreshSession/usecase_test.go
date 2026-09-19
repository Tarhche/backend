package refreshsession

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/khanzadimahdi/testproject/application/auth"
	"github.com/khanzadimahdi/testproject/application/auth/refresh"
	"github.com/khanzadimahdi/testproject/application/oauth"
	"github.com/khanzadimahdi/testproject/domain"
	"github.com/khanzadimahdi/testproject/domain/oauth/client"
	"github.com/khanzadimahdi/testproject/domain/role"
	"github.com/khanzadimahdi/testproject/domain/user"
	"github.com/khanzadimahdi/testproject/infrastructure/crypto/argon2"
	"github.com/khanzadimahdi/testproject/infrastructure/crypto/ecdsa"
	"github.com/khanzadimahdi/testproject/infrastructure/jwt"
	clients "github.com/khanzadimahdi/testproject/infrastructure/repository/mocks/oauth/clients"
	"github.com/khanzadimahdi/testproject/infrastructure/repository/mocks/roles"
	"github.com/khanzadimahdi/testproject/infrastructure/repository/mocks/users"
	"github.com/khanzadimahdi/testproject/infrastructure/translator"
	"github.com/khanzadimahdi/testproject/infrastructure/validator"
)

const userUUID = "user-uuid"

// session is the refresh use case this one leans on, and a refresh token it
// will take.
func session(t *testing.T, u user.User) (*refresh.UseCase, string) {
	t.Helper()

	privateKey, err := ecdsa.Generate()
	require.NoError(t, err)

	j := jwt.NewJWT(privateKey, privateKey.Public())

	var roleRepository roles.MockRolesRepository
	roleRepository.On("GetByUserUUID", mock.Anything, userUUID).Return([]role.Role{}, nil).Maybe()

	generator := auth.NewTokenGenerator(j, &roleRepository)

	token, err := generator.GenerateRefreshToken(context.Background(), userUUID)
	require.NoError(t, err)

	var userRepository users.MockUsersRepository
	userRepository.On("GetOne", mock.Anything, userUUID).Return(u, nil).Maybe()

	var authorizer domain.MockAuthorizer

	var translated translator.TranslatorMock
	translated.On("Translate", mock.Anything, mock.Anything).Return("").Maybe()
	translated.On("Translate", mock.Anything).Return("").Maybe()

	var validated validator.MockValidator
	validated.On("Validate", mock.Anything).Return(nil).Maybe()

	return refresh.NewUseCase(&userRepository, j, generator, &authorizer, &translated, &validated), token
}

func registered(grants ...string) client.Client {
	return client.Client{
		ID:                      "client-id",
		GrantTypes:              grants,
		TokenEndpointAuthMethod: client.AuthMethodNone,
	}
}

func TestUseCase_Execute(t *testing.T) {
	t.Parallel()

	h := argon2.NewArgon2id(1, 8*1024, 1, 32)

	t.Run("a session is renewed", func(t *testing.T) {
		t.Parallel()

		useCase, token := session(t, user.User{UUID: userUUID})

		var clientRepository clients.MockClientsRepository
		clientRepository.On("GetOne", mock.Anything, "client-id").Return(registered(client.GrantAuthorizationCode, client.GrantRefreshToken), nil).Once()

		response, err := NewUseCase(oauth.NewClients(&clientRepository, h), useCase).Execute(context.Background(), &Request{
			RefreshToken: token,
			ClientID:     "client-id",
		})

		require.NoError(t, err)
		assert.NotEmpty(t, response.AccessToken)
		assert.NotEmpty(t, response.RefreshToken)
		assert.Equal(t, oauth.Scope, response.Scope)
	})

	t.Run("an application that did not register for it does not get it", func(t *testing.T) {
		t.Parallel()

		useCase, token := session(t, user.User{UUID: userUUID})

		var clientRepository clients.MockClientsRepository
		clientRepository.On("GetOne", mock.Anything, "client-id").Return(registered(client.GrantAuthorizationCode), nil).Once()

		_, err := NewUseCase(oauth.NewClients(&clientRepository, h), useCase).Execute(context.Background(), &Request{
			RefreshToken: token,
			ClientID:     "client-id",
		})

		var oauthError *oauth.Error
		require.ErrorAs(t, err, &oauthError)
		assert.Equal(t, oauth.ErrorUnauthorizedClient, oauthError.Code)
	})

	t.Run("a ban handed down ends the session at its next renewal", func(t *testing.T) {
		t.Parallel()

		useCase, token := session(t, user.User{UUID: userUUID, BannedAt: time.Now().Add(-time.Hour)})

		var clientRepository clients.MockClientsRepository
		clientRepository.On("GetOne", mock.Anything, "client-id").Return(registered(client.GrantRefreshToken), nil).Once()

		_, err := NewUseCase(oauth.NewClients(&clientRepository, h), useCase).Execute(context.Background(), &Request{
			RefreshToken: token,
			ClientID:     "client-id",
		})

		var oauthError *oauth.Error
		require.ErrorAs(t, err, &oauthError)
		assert.Equal(t, oauth.ErrorInvalidGrant, oauthError.Code)
	})
}
