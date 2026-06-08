package getlanguages

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/khanzadimahdi/testproject/application/language/resolver"
	"github.com/khanzadimahdi/testproject/domain/language"
	"github.com/khanzadimahdi/testproject/infrastructure/repository/mocks/languages"
)

func TestUseCase_Execute(t *testing.T) {
	t.Parallel()

	t.Run("returns all languages", func(t *testing.T) {
		t.Parallel()

		var (
			languageRepository languages.MockLanguagesRepository
			languageResolver   resolver.MockResolver

			l = []language.Language{
				{Code: "EN", Name: "English"},
				{Code: "FA", Name: "فارسی"},
			}

			defaultLanguage = l[0]

			expectedResponse = Response{
				Items: []languageResponse{
					{Code: l[0].Code, Name: l[0].Name},
					{Code: l[1].Code, Name: l[1].Name},
				},
				DefaultLanguage: languageResponse{Code: defaultLanguage.Code, Name: defaultLanguage.Name},
			}
		)

		languageRepository.On("Count").Once().Return(uint(len(l)), nil)
		languageRepository.On("GetAll", uint(0), uint(len(l))).Once().Return(l, nil)
		defer languageRepository.AssertExpectations(t)

		languageResolver.On("DefaultCode").Once().Return(defaultLanguage.Code, nil)
		languageResolver.On("Resolve", defaultLanguage.Code).Once().Return(defaultLanguage, nil)
		defer languageResolver.AssertExpectations(t)

		response, err := NewUseCase(&languageRepository, &languageResolver).Execute()

		assert.NoError(t, err)
		assert.Equal(t, &expectedResponse, response)
	})

	t.Run("counting languages fails", func(t *testing.T) {
		t.Parallel()

		var (
			languageRepository languages.MockLanguagesRepository
			languageResolver   resolver.MockResolver

			expectedError = errors.New("counting failed")
		)

		languageRepository.On("Count").Once().Return(uint(0), expectedError)
		defer languageRepository.AssertExpectations(t)

		response, err := NewUseCase(&languageRepository, &languageResolver).Execute()

		languageRepository.AssertNotCalled(t, "GetAll")
		languageResolver.AssertNotCalled(t, "DefaultCode")
		languageResolver.AssertNotCalled(t, "Resolve")
		assert.Nil(t, response)
		assert.ErrorIs(t, err, expectedError)
	})

	t.Run("getting languages fails", func(t *testing.T) {
		t.Parallel()

		var (
			languageRepository languages.MockLanguagesRepository
			languageResolver   resolver.MockResolver

			expectedError = errors.New("getting failed")
		)

		languageRepository.On("Count").Once().Return(uint(2), nil)
		languageRepository.On("GetAll", uint(0), uint(2)).Once().Return(nil, expectedError)
		defer languageRepository.AssertExpectations(t)

		response, err := NewUseCase(&languageRepository, &languageResolver).Execute()

		languageResolver.AssertNotCalled(t, "DefaultCode")
		languageResolver.AssertNotCalled(t, "Resolve")
		assert.Nil(t, response)
		assert.ErrorIs(t, err, expectedError)
	})

	t.Run("getting default language code fails", func(t *testing.T) {
		t.Parallel()

		var (
			languageRepository languages.MockLanguagesRepository
			languageResolver   resolver.MockResolver

			l = []language.Language{
				{Code: "EN", Name: "English"},
			}

			expectedError = errors.New("resolving default code failed")
		)

		languageRepository.On("Count").Once().Return(uint(len(l)), nil)
		languageRepository.On("GetAll", uint(0), uint(len(l))).Once().Return(l, nil)
		defer languageRepository.AssertExpectations(t)

		languageResolver.On("DefaultCode").Once().Return("", expectedError)
		defer languageResolver.AssertExpectations(t)

		response, err := NewUseCase(&languageRepository, &languageResolver).Execute()

		languageResolver.AssertNotCalled(t, "Resolve")
		assert.Nil(t, response)
		assert.ErrorIs(t, err, expectedError)
	})

	t.Run("resolving default language fails", func(t *testing.T) {
		t.Parallel()

		var (
			languageRepository languages.MockLanguagesRepository
			languageResolver   resolver.MockResolver

			l = []language.Language{
				{Code: "EN", Name: "English"},
			}

			expectedError = errors.New("resolving default language failed")
		)

		languageRepository.On("Count").Once().Return(uint(len(l)), nil)
		languageRepository.On("GetAll", uint(0), uint(len(l))).Once().Return(l, nil)
		defer languageRepository.AssertExpectations(t)

		languageResolver.On("DefaultCode").Once().Return("EN", nil)
		languageResolver.On("Resolve", "EN").Once().Return(language.Language{}, expectedError)
		defer languageResolver.AssertExpectations(t)

		response, err := NewUseCase(&languageRepository, &languageResolver).Execute()

		assert.Nil(t, response)
		assert.ErrorIs(t, err, expectedError)
	})
}
