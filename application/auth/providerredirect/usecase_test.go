package providerredirect

import (
	"context"
	"net/url"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/khanzadimahdi/testproject/domain"
	"github.com/khanzadimahdi/testproject/domain/oauth"
	oauthMock "github.com/khanzadimahdi/testproject/infrastructure/oauth/mock"
	"github.com/khanzadimahdi/testproject/infrastructure/translator"
	"github.com/khanzadimahdi/testproject/infrastructure/validator"
)

func TestUseCase_Execute(t *testing.T) {
	t.Parallel()

	t.Run("where to send somebody, and what to compare when they come back", func(t *testing.T) {
		t.Parallel()

		var (
			googleProvider   oauthMock.MockProvider
			requestValidator validator.MockValidator
			translatorMock   translator.TranslatorMock

			request = Request{Provider: "google"}
		)

		requestValidator.On("Validate", &request).Once().Return(nil)
		defer requestValidator.AssertExpectations(t)

		googleProvider.On("AuthorizationURL", mock.AnythingOfType("string")).Once().
			Return("https://accounts.google.com/o/oauth2/v2/auth?state=given").
			Run(func(args mock.Arguments) {
				assert.NotEmpty(t, args.String(0), "a state nobody can guess is the point of having one")
			})
		defer googleProvider.AssertExpectations(t)

		response, err := NewUseCase(
			oauth.Providers{"google": &googleProvider},
			&translatorMock,
			&requestValidator,
		).Execute(context.Background(), &request)

		require.NoError(t, err)
		assert.Empty(t, response.ValidationErrors)
		assert.NotEmpty(t, response.State)

		address, err := url.Parse(response.URL)
		require.NoError(t, err)
		assert.Equal(t, "accounts.google.com", address.Host)

		translatorMock.AssertNotCalled(t, "Translate")
	})

	t.Run("every state is its own", func(t *testing.T) {
		t.Parallel()

		var (
			googleProvider   oauthMock.MockProvider
			requestValidator validator.MockValidator
			translatorMock   translator.TranslatorMock

			request = Request{Provider: "google"}
		)

		requestValidator.On("Validate", &request).Return(nil)
		googleProvider.On("AuthorizationURL", mock.AnythingOfType("string")).Return("https://accounts.google.com/o/oauth2/v2/auth")

		useCase := NewUseCase(oauth.Providers{"google": &googleProvider}, &translatorMock, &requestValidator)

		first, err := useCase.Execute(context.Background(), &request)
		require.NoError(t, err)

		second, err := useCase.Execute(context.Background(), &request)
		require.NoError(t, err)

		assert.NotEqual(t, first.State, second.State, "a state reused is a state that proves nothing")
	})

	t.Run("a provider nobody here offers", func(t *testing.T) {
		t.Parallel()

		var (
			requestValidator validator.MockValidator
			translatorMock   translator.TranslatorMock

			request = Request{Provider: "myspace"}
		)

		requestValidator.On("Validate", &request).Once().Return(nil)
		defer requestValidator.AssertExpectations(t)

		translatorMock.On("Translate", "unknown_login_provider", mock.Anything).Once().Return("not offered here")
		defer translatorMock.AssertExpectations(t)

		response, err := NewUseCase(oauth.Providers{}, &translatorMock, &requestValidator).
			Execute(context.Background(), &request)

		require.NoError(t, err)
		assert.Equal(t, domain.ValidationErrors{"provider": "not offered here"}, response.ValidationErrors)
		assert.Empty(t, response.URL)
	})

	t.Run("validation fails", func(t *testing.T) {
		t.Parallel()

		var (
			requestValidator validator.MockValidator
			translatorMock   translator.TranslatorMock

			request = Request{}

			expected = domain.ValidationErrors{"provider": "this field is required"}
		)

		requestValidator.On("Validate", &request).Once().Return(expected)
		defer requestValidator.AssertExpectations(t)

		response, err := NewUseCase(oauth.Providers{}, &translatorMock, &requestValidator).
			Execute(context.Background(), &request)

		require.NoError(t, err)
		assert.Equal(t, expected, response.ValidationErrors)
	})
}

func TestRequest_Validate(t *testing.T) {
	t.Parallel()

	assert.Equal(t, domain.ValidationErrors{}, (&Request{Provider: "google"}).Validate())
	assert.Equal(t, domain.ValidationErrors{"provider": "required_field"}, (&Request{}).Validate())
}
