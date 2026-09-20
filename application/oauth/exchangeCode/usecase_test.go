package exchangecode

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/khanzadimahdi/testproject/application/auth"
	"github.com/khanzadimahdi/testproject/application/oauth"
	"github.com/khanzadimahdi/testproject/domain"
	"github.com/khanzadimahdi/testproject/domain/oauth/client"
	"github.com/khanzadimahdi/testproject/domain/oauth/grant"
	"github.com/khanzadimahdi/testproject/domain/password"
	"github.com/khanzadimahdi/testproject/domain/role"
	"github.com/khanzadimahdi/testproject/domain/user"
	"github.com/khanzadimahdi/testproject/infrastructure/crypto/argon2"
	"github.com/khanzadimahdi/testproject/infrastructure/crypto/ecdsa"
	"github.com/khanzadimahdi/testproject/infrastructure/jwt"
	clients "github.com/khanzadimahdi/testproject/infrastructure/repository/mocks/oauth/clients"
	grants "github.com/khanzadimahdi/testproject/infrastructure/repository/mocks/oauth/grants"
	"github.com/khanzadimahdi/testproject/infrastructure/repository/mocks/roles"
	"github.com/khanzadimahdi/testproject/infrastructure/repository/mocks/users"
)

const (
	userUUID = "user-uuid"
	verifier = "a-verifier-long-enough-to-be-one"
	secret   = "the-grant-secret"
)

func hasher() password.Hasher {
	return argon2.NewArgon2id(1, 8*1024, 1, 32)
}

func challenge() string {
	sum := sha256.Sum256([]byte(verifier))

	return base64.RawURLEncoding.EncodeToString(sum[:])
}

func tokens(t *testing.T) *auth.AuthTokenGenerator {
	t.Helper()

	privateKey, err := ecdsa.Generate()
	require.NoError(t, err)

	var roleRepository roles.MockRolesRepository
	roleRepository.On("GetByUserUUID", mock.Anything, userUUID).Return([]role.Role{{Name: "writer", Permissions: []string{"articles.index"}}}, nil).Maybe()

	return auth.NewTokenGenerator(jwt.NewJWT(privateKey, privateKey.Public()), &roleRepository)
}

func granted(t *testing.T, h password.Hasher) grant.Grant {
	t.Helper()

	salt := []byte("salt")

	return grant.Grant{
		ID:                  "grant-id",
		Secret:              password.Hash{Value: h.Hash(context.Background(), []byte(secret), salt), Salt: salt},
		ClientID:            "client-id",
		UserUUID:            userUUID,
		RedirectURI:         "https://agent.example/callback",
		Scope:               oauth.Scope,
		CodeChallenge:       challenge(),
		CodeChallengeMethod: grant.ChallengeMethodS256,
		ExpiredAt:           time.Now().Add(time.Minute),
	}
}

func public() client.Client {
	return client.Client{
		ID:                      "client-id",
		RedirectURIs:            []string{"https://agent.example/callback"},
		GrantTypes:              []string{client.GrantAuthorizationCode, client.GrantRefreshToken},
		TokenEndpointAuthMethod: client.AuthMethodNone,
	}
}

func TestUseCase_Execute(t *testing.T) {
	t.Parallel()

	t.Run("a code, answered for, is a session", func(t *testing.T) {
		t.Parallel()

		h := hasher()

		var clientRepository clients.MockClientsRepository
		clientRepository.On("GetOne", mock.Anything, "client-id").Return(public(), nil).Once()
		defer clientRepository.AssertExpectations(t)

		var grantRepository grants.MockGrantsRepository
		grantRepository.On("Consume", mock.Anything, "grant-id").Return(granted(t, h), nil).Once()
		defer grantRepository.AssertExpectations(t)

		var userRepository users.MockUsersRepository
		userRepository.On("GetOne", mock.Anything, userUUID).Return(user.User{UUID: userUUID}, nil).Once()
		defer userRepository.AssertExpectations(t)

		response, err := NewUseCase(&grantRepository, oauth.NewClients(&clientRepository, h), &userRepository, h, tokens(t)).
			Execute(context.Background(), &Request{
				Code:         "grant-id." + secret,
				ClientID:     "client-id",
				RedirectURI:  "https://agent.example/callback",
				CodeVerifier: verifier,
			})

		require.NoError(t, err)
		assert.NotEmpty(t, response.AccessToken)
		assert.NotEmpty(t, response.RefreshToken)
		assert.Equal(t, "Bearer", response.TokenType)
		assert.Equal(t, int(auth.AccessTokenExpirationTime.Seconds()), response.ExpiresIn)
		assert.Equal(t, oauth.Scope, response.Scope)
	})

	t.Run("a code without the verifier it was asked for is worth nothing", func(t *testing.T) {
		t.Parallel()

		h := hasher()

		var clientRepository clients.MockClientsRepository
		clientRepository.On("GetOne", mock.Anything, "client-id").Return(public(), nil).Once()

		var grantRepository grants.MockGrantsRepository
		grantRepository.On("Consume", mock.Anything, "grant-id").Return(granted(t, h), nil).Once()

		var userRepository users.MockUsersRepository
		defer userRepository.AssertExpectations(t)

		_, err := NewUseCase(&grantRepository, oauth.NewClients(&clientRepository, h), &userRepository, h, tokens(t)).
			Execute(context.Background(), &Request{
				Code:         "grant-id." + secret,
				ClientID:     "client-id",
				RedirectURI:  "https://agent.example/callback",
				CodeVerifier: "not-the-verifier-the-challenge-was-made-from",
			})

		var oauthError *oauth.Error
		require.ErrorAs(t, err, &oauthError)
		assert.Equal(t, oauth.ErrorInvalidGrant, oauthError.Code)
	})

	t.Run("a code that was already collected is gone", func(t *testing.T) {
		t.Parallel()

		h := hasher()

		var clientRepository clients.MockClientsRepository
		clientRepository.On("GetOne", mock.Anything, "client-id").Return(public(), nil).Once()

		var grantRepository grants.MockGrantsRepository
		grantRepository.On("Consume", mock.Anything, "grant-id").Return(grant.Grant{}, domain.ErrNotExists).Once()

		var userRepository users.MockUsersRepository

		_, err := NewUseCase(&grantRepository, oauth.NewClients(&clientRepository, h), &userRepository, h, tokens(t)).
			Execute(context.Background(), &Request{
				Code:         "grant-id." + secret,
				ClientID:     "client-id",
				CodeVerifier: verifier,
			})

		var oauthError *oauth.Error
		require.ErrorAs(t, err, &oauthError)
		assert.Equal(t, oauth.ErrorInvalidGrant, oauthError.Code)
	})

	t.Run("a code that waited too long is gone", func(t *testing.T) {
		t.Parallel()

		h := hasher()

		expired := granted(t, h)
		expired.ExpiredAt = time.Now().Add(-time.Second)

		var clientRepository clients.MockClientsRepository
		clientRepository.On("GetOne", mock.Anything, "client-id").Return(public(), nil).Once()

		var grantRepository grants.MockGrantsRepository
		grantRepository.On("Consume", mock.Anything, "grant-id").Return(expired, nil).Once()

		var userRepository users.MockUsersRepository

		_, err := NewUseCase(&grantRepository, oauth.NewClients(&clientRepository, h), &userRepository, h, tokens(t)).
			Execute(context.Background(), &Request{
				Code:         "grant-id." + secret,
				ClientID:     "client-id",
				CodeVerifier: verifier,
			})

		var oauthError *oauth.Error
		require.ErrorAs(t, err, &oauthError)
		assert.Equal(t, oauth.ErrorInvalidGrant, oauthError.Code)
	})

	t.Run("a code belongs to the application it was given to", func(t *testing.T) {
		t.Parallel()

		h := hasher()

		somebodyElse := public()
		somebodyElse.ID = "another-client"

		var clientRepository clients.MockClientsRepository
		clientRepository.On("GetOne", mock.Anything, "another-client").Return(somebodyElse, nil).Once()

		var grantRepository grants.MockGrantsRepository
		grantRepository.On("Consume", mock.Anything, "grant-id").Return(granted(t, h), nil).Once()

		var userRepository users.MockUsersRepository

		_, err := NewUseCase(&grantRepository, oauth.NewClients(&clientRepository, h), &userRepository, h, tokens(t)).
			Execute(context.Background(), &Request{
				Code:         "grant-id." + secret,
				ClientID:     "another-client",
				CodeVerifier: verifier,
			})

		var oauthError *oauth.Error
		require.ErrorAs(t, err, &oauthError)
		assert.Equal(t, oauth.ErrorInvalidGrant, oauthError.Code)
	})

	t.Run("a banned account hands over nothing", func(t *testing.T) {
		t.Parallel()

		h := hasher()

		var clientRepository clients.MockClientsRepository
		clientRepository.On("GetOne", mock.Anything, "client-id").Return(public(), nil).Once()

		var grantRepository grants.MockGrantsRepository
		grantRepository.On("Consume", mock.Anything, "grant-id").Return(granted(t, h), nil).Once()

		var userRepository users.MockUsersRepository
		userRepository.On("GetOne", mock.Anything, userUUID).Return(user.User{UUID: userUUID, BannedAt: time.Now().Add(-time.Hour)}, nil).Once()

		_, err := NewUseCase(&grantRepository, oauth.NewClients(&clientRepository, h), &userRepository, h, tokens(t)).
			Execute(context.Background(), &Request{
				Code:         "grant-id." + secret,
				ClientID:     "client-id",
				CodeVerifier: verifier,
			})

		var oauthError *oauth.Error
		require.ErrorAs(t, err, &oauthError)
		assert.Equal(t, oauth.ErrorInvalidGrant, oauthError.Code)
	})

	t.Run("an application with a secret has to show it, before the code is spent", func(t *testing.T) {
		t.Parallel()

		h := hasher()
		salt := []byte("salt")

		confidential := public()
		confidential.TokenEndpointAuthMethod = client.AuthMethodSecretPost
		confidential.Secret = password.Hash{Value: h.Hash(context.Background(), []byte("the-client-secret"), salt), Salt: salt}

		var clientRepository clients.MockClientsRepository
		clientRepository.On("GetOne", mock.Anything, "client-id").Return(confidential, nil).Once()

		// nothing is consumed: who is asking is settled first
		var grantRepository grants.MockGrantsRepository
		defer grantRepository.AssertExpectations(t)

		var userRepository users.MockUsersRepository

		_, err := NewUseCase(&grantRepository, oauth.NewClients(&clientRepository, h), &userRepository, h, tokens(t)).
			Execute(context.Background(), &Request{
				Code:         "grant-id." + secret,
				ClientID:     "client-id",
				ClientSecret: "not-the-secret",
				CodeVerifier: verifier,
			})

		var oauthError *oauth.Error
		require.ErrorAs(t, err, &oauthError)
		assert.Equal(t, oauth.ErrorInvalidClient, oauthError.Code)
	})
}
