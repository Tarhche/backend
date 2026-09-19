package auth

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/khanzadimahdi/testproject/application/auth/providerredirect"
	authProviders "github.com/khanzadimahdi/testproject/application/auth/providers"
	"github.com/khanzadimahdi/testproject/domain/oauth"
	oauthMock "github.com/khanzadimahdi/testproject/infrastructure/oauth/mock"
	"github.com/khanzadimahdi/testproject/infrastructure/translator"
	"github.com/khanzadimahdi/testproject/infrastructure/validator"
)

func TestProvidersHandler(t *testing.T) {
	t.Parallel()

	t.Run("what may be signed in with", func(t *testing.T) {
		t.Parallel()

		var google, github oauthMock.MockProvider

		handler := NewProvidersHandler(authProviders.NewUseCase(oauth.Providers{
			"google": &google,
			"github": &github,
		}))

		request := httptest.NewRequest(http.MethodGet, "/", nil)
		response := httptest.NewRecorder()

		handler.ServeHTTP(response, request)

		expectedBody, err := os.ReadFile("testdata/oauth-providers-response.json")
		require.NoError(t, err)

		assert.Equal(t, "application/json", response.Header().Get("content-type"))
		assert.JSONEq(t, string(expectedBody), response.Body.String())
		assert.Equal(t, http.StatusOK, response.Code)
	})
}

func TestProviderRedirectHandler(t *testing.T) {
	t.Parallel()

	t.Run("where to send the browser", func(t *testing.T) {
		t.Parallel()

		var (
			googleProvider   oauthMock.MockProvider
			requestValidator validator.MockValidator
			translatorMock   translator.TranslatorMock
		)

		requestValidator.On("Validate", mock.Anything).Once().Return(nil)
		defer requestValidator.AssertExpectations(t)

		googleProvider.On("AuthorizationURL", mock.AnythingOfType("string")).Once().
			Return("https://accounts.google.com/o/oauth2/v2/auth?client_id=x")
		defer googleProvider.AssertExpectations(t)

		handler := NewProviderRedirectHandler(providerredirect.NewUseCase(
			oauth.Providers{"google": &googleProvider},
			&translatorMock,
			&requestValidator,
		))

		request := httptest.NewRequest(http.MethodGet, "/", nil)
		request.SetPathValue("provider", "google")
		response := httptest.NewRecorder()

		handler.ServeHTTP(response, request)

		assert.Equal(t, "application/json", response.Header().Get("content-type"))
		assert.Equal(t, http.StatusOK, response.Code)

		var body providerredirect.Response
		require.NoError(t, json.NewDecoder(response.Body).Decode(&body))

		assert.Equal(t, "https://accounts.google.com/o/oauth2/v2/auth?client_id=x", body.URL)
		assert.NotEmpty(t, body.State, "the caller keeps this and compares it when the browser comes back")
	})

	t.Run("a provider nobody here offers", func(t *testing.T) {
		t.Parallel()

		var (
			requestValidator validator.MockValidator
			translatorMock   translator.TranslatorMock
		)

		requestValidator.On("Validate", mock.Anything).Once().Return(nil)
		defer requestValidator.AssertExpectations(t)

		translatorMock.On("Translate", "unknown_login_provider", mock.Anything).Once().
			Return("signing in that way is not offered here")
		defer translatorMock.AssertExpectations(t)

		handler := NewProviderRedirectHandler(providerredirect.NewUseCase(
			oauth.Providers{},
			&translatorMock,
			&requestValidator,
		))

		request := httptest.NewRequest(http.MethodGet, "/", nil)
		request.SetPathValue("provider", "myspace")
		response := httptest.NewRecorder()

		handler.ServeHTTP(response, request)

		expectedBody, err := os.ReadFile("testdata/oauth-unknown-provider-response.json")
		require.NoError(t, err)

		assert.Equal(t, "application/json", response.Header().Get("content-type"))
		assert.JSONEq(t, string(expectedBody), response.Body.String())
		assert.Equal(t, http.StatusBadRequest, response.Code)
	})
}
