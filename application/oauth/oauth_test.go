package oauth

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/khanzadimahdi/testproject/domain"
	"github.com/khanzadimahdi/testproject/domain/oauth/client"
	"github.com/khanzadimahdi/testproject/domain/password"
	"github.com/khanzadimahdi/testproject/infrastructure/crypto/argon2"
	"github.com/khanzadimahdi/testproject/infrastructure/crypto/ecdsa"
	"github.com/khanzadimahdi/testproject/infrastructure/jwt"
	clients "github.com/khanzadimahdi/testproject/infrastructure/repository/mocks/oauth/clients"
)

func TestRequests(t *testing.T) {
	t.Parallel()

	privateKey, err := ecdsa.Generate()
	require.NoError(t, err)

	requests := NewRequests(jwt.NewJWT(privateKey, privateKey.Public()))

	asked := AuthorizationRequest{
		ClientID:            "client-id",
		ClientName:          "An agent",
		ClientURI:           "https://agent.example",
		RedirectURI:         "https://agent.example/callback",
		State:               "state",
		Scope:               Scope,
		CodeChallenge:       "a-challenge",
		CodeChallengeMethod: "S256",
		Resource:            "https://api.example/mcp",
	}

	t.Run("a request comes back as it went", func(t *testing.T) {
		token, err := requests.Generate(context.Background(), asked)
		require.NoError(t, err)

		read, err := requests.Verify(context.Background(), token)
		require.NoError(t, err)
		assert.Equal(t, asked, read)
	})

	t.Run("a token for something else is not a request", func(t *testing.T) {
		claims := jwt.NewClaimsBuilder()
		claims.SetSubject("client-id")
		claims.SetAudience([]string{"permission"})

		token, err := jwt.NewJWT(privateKey, privateKey.Public()).Generate(context.Background(), claims.Build())
		require.NoError(t, err)

		_, err = requests.Verify(context.Background(), token)

		var oauthError *Error
		require.ErrorAs(t, err, &oauthError)
		assert.Equal(t, ErrorInvalidRequest, oauthError.Code)
	})
}

func TestAnswersChallenge(t *testing.T) {
	t.Parallel()

	verifier := "the-verifier-this-was-made-from"
	sum := sha256.Sum256([]byte(verifier))
	challenge := base64.RawURLEncoding.EncodeToString(sum[:])

	assert.True(t, AnswersChallenge(challenge, verifier))
	assert.False(t, AnswersChallenge(challenge, "something-else"))
	assert.False(t, AnswersChallenge("", verifier))
	assert.False(t, AnswersChallenge(challenge, ""))
}

func TestClients_Authenticate(t *testing.T) {
	t.Parallel()

	h := argon2.NewArgon2id(1, 8*1024, 1, 32)
	salt := []byte("salt")

	confidential := client.Client{
		ID:                      "client-id",
		TokenEndpointAuthMethod: client.AuthMethodSecretPost,
		Secret:                  password.Hash{Value: h.Hash(context.Background(), []byte("the-secret"), salt), Salt: salt},
	}

	t.Run("a public application is taken at its word", func(t *testing.T) {
		var repository clients.MockClientsRepository
		repository.On("GetOne", mock.Anything, "client-id").Return(client.Client{ID: "client-id", TokenEndpointAuthMethod: client.AuthMethodNone}, nil).Once()

		registered, err := NewClients(&repository, h).Authenticate(context.Background(), "client-id", "")
		require.NoError(t, err)
		assert.Equal(t, "client-id", registered.ID)
	})

	t.Run("an application with a secret shows it", func(t *testing.T) {
		var repository clients.MockClientsRepository
		repository.On("GetOne", mock.Anything, "client-id").Return(confidential, nil).Twice()

		registered, err := NewClients(&repository, h).Authenticate(context.Background(), "client-id", "the-secret")
		require.NoError(t, err)
		assert.Equal(t, "client-id", registered.ID)

		_, err = NewClients(&repository, h).Authenticate(context.Background(), "client-id", "not-the-secret")

		var oauthError *Error
		require.ErrorAs(t, err, &oauthError)
		assert.Equal(t, ErrorInvalidClient, oauthError.Code)
	})

	t.Run("an application nobody registered is nobody", func(t *testing.T) {
		var repository clients.MockClientsRepository
		repository.On("GetOne", mock.Anything, "client-id").Return(client.Client{}, domain.ErrNotExists).Once()

		_, err := NewClients(&repository, h).Authenticate(context.Background(), "client-id", "")

		var oauthError *Error
		require.ErrorAs(t, err, &oauthError)
		assert.Equal(t, ErrorInvalidClient, oauthError.Code)

		_, err = NewClients(&repository, h).Authenticate(context.Background(), "", "")
		require.ErrorAs(t, err, &oauthError)
		assert.Equal(t, ErrorInvalidClient, oauthError.Code)
	})
}
