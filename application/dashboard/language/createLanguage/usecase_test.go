package createlanguage

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"

	"github.com/khanzadimahdi/testproject/domain"
	"github.com/khanzadimahdi/testproject/infrastructure/repository/mocks/languages"
	"github.com/khanzadimahdi/testproject/infrastructure/validator"
)

func TestUseCase_Execute(t *testing.T) {
	t.Parallel()

	t.Run("creates a language", func(t *testing.T) {
		t.Parallel()

		var (
			languageRepository languages.MockLanguagesRepository
			requestValidator   validator.MockValidator

			request = Request{Code: "DE", Name: "Deutsch"}
		)

		requestValidator.On("Validate", &request).Once().Return(nil)
		defer requestValidator.AssertExpectations(t)

		languageRepository.On("Exists", request.Code).Once().Return(false)
		languageRepository.On("Save", mock.AnythingOfType("*language.Language")).Once().Return(request.Code, nil)
		defer languageRepository.AssertExpectations(t)

		response, err := NewUseCase(&languageRepository, &requestValidator).Execute(&request)

		assert.NoError(t, err)
		assert.NotNil(t, response)
		assert.Empty(t, response.ValidationErrors)
		assert.Equal(t, request.Code, response.Code)
	})

	t.Run("validation fails", func(t *testing.T) {
		t.Parallel()

		var (
			languageRepository languages.MockLanguagesRepository
			requestValidator   validator.MockValidator

			request          = Request{}
			validationErrors = domain.ValidationErrors{
				"code": "required_field",
				"name": "required_field",
			}
		)

		requestValidator.On("Validate", &request).Once().Return(validationErrors)
		defer requestValidator.AssertExpectations(t)

		response, err := NewUseCase(&languageRepository, &requestValidator).Execute(&request)

		languageRepository.AssertNotCalled(t, "Exists")
		languageRepository.AssertNotCalled(t, "Save")

		assert.NoError(t, err)
		assert.Equal(t, validationErrors, response.ValidationErrors)
	})

	t.Run("language already exists", func(t *testing.T) {
		t.Parallel()

		var (
			languageRepository languages.MockLanguagesRepository
			requestValidator   validator.MockValidator

			request = Request{Code: "EN", Name: "English"}
		)

		requestValidator.On("Validate", &request).Once().Return(nil)
		defer requestValidator.AssertExpectations(t)

		languageRepository.On("Exists", request.Code).Once().Return(true)
		defer languageRepository.AssertExpectations(t)

		response, err := NewUseCase(&languageRepository, &requestValidator).Execute(&request)

		languageRepository.AssertNotCalled(t, "Save")

		assert.NoError(t, err)
		assert.Equal(t, "already_exists", response.ValidationErrors["code"])
	})

	t.Run("saving fails", func(t *testing.T) {
		t.Parallel()

		var (
			languageRepository languages.MockLanguagesRepository
			requestValidator   validator.MockValidator

			request       = Request{Code: "DE", Name: "Deutsch"}
			expectedError = errors.New("saving failed")
		)

		requestValidator.On("Validate", &request).Once().Return(nil)
		defer requestValidator.AssertExpectations(t)

		languageRepository.On("Exists", request.Code).Once().Return(false)
		languageRepository.On("Save", mock.AnythingOfType("*language.Language")).Once().Return("", expectedError)
		defer languageRepository.AssertExpectations(t)

		response, err := NewUseCase(&languageRepository, &requestValidator).Execute(&request)

		assert.Nil(t, response)
		assert.ErrorIs(t, err, expectedError)
	})
}
