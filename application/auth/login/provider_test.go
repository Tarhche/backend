package login

import (
	"context"
	"errors"
	"testing"
	"time"

	jwtv5 "github.com/golang-jwt/jwt/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/khanzadimahdi/testproject/application/auth"
	"github.com/khanzadimahdi/testproject/application/language/resolver"
	"github.com/khanzadimahdi/testproject/domain"
	"github.com/khanzadimahdi/testproject/domain/oauth"
	"github.com/khanzadimahdi/testproject/domain/role"
	"github.com/khanzadimahdi/testproject/domain/user"
	"github.com/khanzadimahdi/testproject/infrastructure/crypto/ecdsa"
	"github.com/khanzadimahdi/testproject/infrastructure/jwt"
	oauthMock "github.com/khanzadimahdi/testproject/infrastructure/oauth/mock"
	configMocks "github.com/khanzadimahdi/testproject/infrastructure/repository/mocks/config"
	"github.com/khanzadimahdi/testproject/infrastructure/repository/mocks/roles"
	"github.com/khanzadimahdi/testproject/infrastructure/repository/mocks/users"
	"github.com/khanzadimahdi/testproject/infrastructure/translator"
	"github.com/khanzadimahdi/testproject/infrastructure/validator"
)

// TestUseCase_ExecuteWithProvider covers the other way of proving who you are:
// the request carries no password at all, and what it does carry is worth
// nothing until the provider says whose it is.
func TestUseCase_ExecuteWithProvider(t *testing.T) {
	t.Parallel()

	privateKey, err := ecdsa.Generate()
	require.NoError(t, err)

	j := jwt.NewJWT(privateKey, privateKey.Public())

	identity := oauth.Identity{
		Provider: "google",
		ID:       "google-subject",
		Email:    "some.body@example.com",
		Verified: true,
		Name:     "Some Body",
	}

	t.Run("somebody the provider vouches for is signed in", func(t *testing.T) {
		t.Parallel()

		var (
			userRepository   users.MockUsersRepository
			roleRepository   roles.MockRolesRepository
			configRepository configMocks.MockConfigRepository
			languageResolver resolver.MockResolver
			googleProvider   oauthMock.MockProvider
			requestValidator validator.MockValidator
			translatorMock   translator.TranslatorMock

			u = user.User{UUID: "user-uuid", LanguageCode: "EN"}

			request = Request{Provider: "google", Code: "code-value"}
		)

		requestValidator.On("Validate", &request).Once().Return(nil)
		defer requestValidator.AssertExpectations(t)

		googleProvider.On("Identify", mock.Anything, "code-value").Once().Return(identity, nil)
		defer googleProvider.AssertExpectations(t)

		userRepository.On("GetOneByProviderIdentity", mock.Anything, "google", "google-subject").Once().Return(u, nil)
		defer userRepository.AssertExpectations(t)

		roleRepository.On("GetByUserUUID", mock.Anything, u.UUID).Once().Return([]role.Role{}, nil)

		identities := auth.NewIdentities(&userRepository, &roleRepository, &configRepository, &languageResolver)
		providers := oauth.Providers{"google": &googleProvider}

		response, err := NewUseCase(
			&userRepository,
			auth.NewTokenGenerator(j, &roleRepository),
			identities,
			providers,
			nil,
			&translatorMock,
			&requestValidator,
		).Execute(context.Background(), &request)

		require.NoError(t, err)
		require.NotNil(t, response)
		assert.Empty(t, response.ValidationErrors)

		claims, err := j.Verify(context.Background(), response.AccessToken)
		require.NoError(t, err)

		subject, err := claims.GetSubject()
		require.NoError(t, err)
		assert.Equal(t, u.UUID, subject)

		audience, err := claims.GetAudience()
		require.NoError(t, err)
		assert.Equal(t, auth.AccessToken, audience[0])

		claimsMap, ok := claims.(jwtv5.MapClaims)
		require.True(t, ok)
		assert.Equal(t, u.LanguageCode, claimsMap["lang"])

		// nothing about the password path is touched: there is no password
		userRepository.AssertNotCalled(t, "GetOneByIdentity", mock.Anything, mock.Anything)
		translatorMock.AssertNotCalled(t, "Translate")
	})

	t.Run("a provider nobody here offers", func(t *testing.T) {
		t.Parallel()

		var (
			userRepository   users.MockUsersRepository
			roleRepository   roles.MockRolesRepository
			googleProvider   oauthMock.MockProvider
			requestValidator validator.MockValidator
			translatorMock   translator.TranslatorMock

			request = Request{Provider: "myspace", Code: "code-value"}
		)

		requestValidator.On("Validate", &request).Once().Return(nil)
		defer requestValidator.AssertExpectations(t)

		translatorMock.On("Translate", "unknown_login_provider", mock.Anything).Once().Return("not offered here")
		defer translatorMock.AssertExpectations(t)

		response, err := NewUseCase(
			&userRepository,
			auth.NewTokenGenerator(j, &roleRepository),
			nil,
			oauth.Providers{"google": &googleProvider},
			nil,
			&translatorMock,
			&requestValidator,
		).Execute(context.Background(), &request)

		require.NoError(t, err)
		assert.Equal(t, domain.ValidationErrors{"provider": "not offered here"}, response.ValidationErrors)
		assert.Empty(t, response.AccessToken)

		googleProvider.AssertNotCalled(t, "Identify", mock.Anything, mock.Anything)
	})

	t.Run("a code the provider will not trade", func(t *testing.T) {
		t.Parallel()

		var (
			userRepository   users.MockUsersRepository
			roleRepository   roles.MockRolesRepository
			googleProvider   oauthMock.MockProvider
			requestValidator validator.MockValidator
			translatorMock   translator.TranslatorMock

			request = Request{Provider: "google", Code: "spent-code"}
		)

		requestValidator.On("Validate", &request).Once().Return(nil)
		defer requestValidator.AssertExpectations(t)

		googleProvider.On("Identify", mock.Anything, "spent-code").Once().
			Return(oauth.Identity{}, errors.New("the provider refused the authorization code"))
		defer googleProvider.AssertExpectations(t)

		translatorMock.On("Translate", "invalid_login_provider_code", mock.Anything).Once().Return("please try again")
		defer translatorMock.AssertExpectations(t)

		response, err := NewUseCase(
			&userRepository,
			auth.NewTokenGenerator(j, &roleRepository),
			nil,
			oauth.Providers{"google": &googleProvider},
			nil,
			&translatorMock,
			&requestValidator,
		).Execute(context.Background(), &request)

		// a login that did not happen is the caller's to hear about, not a
		// failure of this estate
		require.NoError(t, err)
		assert.Equal(t, domain.ValidationErrors{"code": "please try again"}, response.ValidationErrors)
		assert.Empty(t, response.AccessToken)
	})

	t.Run("an account the provider has not verified an address for", func(t *testing.T) {
		t.Parallel()

		var (
			userRepository   users.MockUsersRepository
			roleRepository   roles.MockRolesRepository
			configRepository configMocks.MockConfigRepository
			languageResolver resolver.MockResolver
			githubProvider   oauthMock.MockProvider
			requestValidator validator.MockValidator
			translatorMock   translator.TranslatorMock

			unverified = oauth.Identity{Provider: "github", ID: "4711", Email: "typed@example.com"}

			request = Request{Provider: "github", Code: "code-value"}
		)

		requestValidator.On("Validate", &request).Once().Return(nil)
		defer requestValidator.AssertExpectations(t)

		githubProvider.On("Identify", mock.Anything, "code-value").Once().Return(unverified, nil)
		defer githubProvider.AssertExpectations(t)

		userRepository.On("GetOneByProviderIdentity", mock.Anything, "github", "4711").Once().Return(user.User{}, domain.ErrNotExists)
		defer userRepository.AssertExpectations(t)

		translatorMock.On("Translate", "provider_email_not_verified", mock.Anything).Once().Return("no verified address")
		defer translatorMock.AssertExpectations(t)

		identities := auth.NewIdentities(&userRepository, &roleRepository, &configRepository, &languageResolver)

		response, err := NewUseCase(
			&userRepository,
			auth.NewTokenGenerator(j, &roleRepository),
			identities,
			oauth.Providers{"github": &githubProvider},
			nil,
			&translatorMock,
			&requestValidator,
		).Execute(context.Background(), &request)

		require.NoError(t, err)
		assert.Equal(t, domain.ValidationErrors{"provider": "no verified address"}, response.ValidationErrors)
		assert.Empty(t, response.AccessToken)
	})

	t.Run("a banned user is banned whichever door they come through", func(t *testing.T) {
		t.Parallel()

		var (
			userRepository   users.MockUsersRepository
			roleRepository   roles.MockRolesRepository
			configRepository configMocks.MockConfigRepository
			languageResolver resolver.MockResolver
			googleProvider   oauthMock.MockProvider
			requestValidator validator.MockValidator
			translatorMock   translator.TranslatorMock

			banned = user.User{UUID: "user-uuid", BannedAt: time.Now().Add(-time.Hour)}

			request = Request{Provider: "google", Code: "code-value"}
		)

		requestValidator.On("Validate", &request).Once().Return(nil)
		defer requestValidator.AssertExpectations(t)

		googleProvider.On("Identify", mock.Anything, "code-value").Once().Return(identity, nil)
		defer googleProvider.AssertExpectations(t)

		userRepository.On("GetOneByProviderIdentity", mock.Anything, "google", "google-subject").Once().Return(banned, nil)
		defer userRepository.AssertExpectations(t)

		translatorMock.On("Translate", "user_is_banned", mock.Anything).Once().Return("your account has been blocked")
		defer translatorMock.AssertExpectations(t)

		identities := auth.NewIdentities(&userRepository, &roleRepository, &configRepository, &languageResolver)

		response, err := NewUseCase(
			&userRepository,
			auth.NewTokenGenerator(j, &roleRepository),
			identities,
			oauth.Providers{"google": &googleProvider},
			nil,
			&translatorMock,
			&requestValidator,
		).Execute(context.Background(), &request)

		require.NoError(t, err)
		assert.Equal(t, domain.ValidationErrors{"identity": "your account has been blocked"}, response.ValidationErrors)
		assert.Empty(t, response.AccessToken)
		assert.Empty(t, response.RefreshToken)

		roleRepository.AssertNotCalled(t, "GetByUserUUID", mock.Anything, mock.Anything)
	})
}

func TestRequest_ValidateWithProvider(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		request Request
		want    domain.ValidationErrors
	}{
		{
			name:    "a provider and the code it handed back",
			request: Request{Provider: "google", Code: "code-value"},
			want:    domain.ValidationErrors{},
		},
		{
			name:    "a provider and nothing from it",
			request: Request{Provider: "google"},
			want:    domain.ValidationErrors{"code": "required_field"},
		},
		{
			// a request that names a provider is not asked for a password, and
			// not having one is not an error
			name:    "a provider needs no password",
			request: Request{Provider: "github", Code: "code-value", Identity: "", Password: ""},
			want:    domain.ValidationErrors{},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, test.want, test.request.Validate())
		})
	}
}
