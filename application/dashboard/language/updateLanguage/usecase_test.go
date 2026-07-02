package updatelanguage

import (
	"context"
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

	t.Run("updates a language", func(t *testing.T) {
		t.Parallel()

		var (
			languageRepository languages.MockLanguagesRepository
			requestValidator   validator.MockValidator

			request = Request{Code: "EN", Name: "English (US)"}
		)

		requestValidator.On("Validate", &request).Once().Return(nil)
		defer requestValidator.AssertExpectations(t)

		languageRepository.On("Exists", mock.Anything, request.Code).Once().Return(true)
		languageRepository.On("Save", mock.Anything, mock.AnythingOfType("*language.Language")).Once().Return(request.Code, nil)
		defer languageRepository.AssertExpectations(t)

		response, err := NewUseCase(&languageRepository, &requestValidator).Execute(context.Background(), &request)

		assert.NoError(t, err)
		assert.Nil(t, response)
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

		response, err := NewUseCase(&languageRepository, &requestValidator).Execute(context.Background(), &request)

		languageRepository.AssertNotCalled(t, "Exists")
		languageRepository.AssertNotCalled(t, "Save")

		assert.NoError(t, err)
		assert.Equal(t, validationErrors, response.ValidationErrors)
	})

	t.Run("language does not exist", func(t *testing.T) {
		t.Parallel()

		var (
			languageRepository languages.MockLanguagesRepository
			requestValidator   validator.MockValidator

			request = Request{Code: "DE", Name: "Deutsch"}
		)

		requestValidator.On("Validate", &request).Once().Return(nil)
		defer requestValidator.AssertExpectations(t)

		languageRepository.On("Exists", mock.Anything, request.Code).Once().Return(false)
		defer languageRepository.AssertExpectations(t)

		response, err := NewUseCase(&languageRepository, &requestValidator).Execute(context.Background(), &request)

		languageRepository.AssertNotCalled(t, "Save")

		assert.Nil(t, response)
		assert.ErrorIs(t, err, domain.ErrNotExists)
	})

	t.Run("saving fails", func(t *testing.T) {
		t.Parallel()

		var (
			languageRepository languages.MockLanguagesRepository
			requestValidator   validator.MockValidator

			request       = Request{Code: "EN", Name: "English"}
			expectedError = errors.New("saving failed")
		)

		requestValidator.On("Validate", &request).Once().Return(nil)
		defer requestValidator.AssertExpectations(t)

		languageRepository.On("Exists", mock.Anything, request.Code).Once().Return(true)
		languageRepository.On("Save", mock.Anything, mock.AnythingOfType("*language.Language")).Once().Return("", expectedError)
		defer languageRepository.AssertExpectations(t)

		response, err := NewUseCase(&languageRepository, &requestValidator).Execute(context.Background(), &request)

		assert.Nil(t, response)
		assert.ErrorIs(t, err, expectedError)
	})
}
