package getlanguages

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/khanzadimahdi/testproject/domain/language"
	"github.com/khanzadimahdi/testproject/infrastructure/repository/mocks/languages"
)

func TestUseCase_Execute(t *testing.T) {
	t.Parallel()

	t.Run("returns all languages", func(t *testing.T) {
		t.Parallel()

		var (
			languageRepository languages.MockLanguagesRepository

			l = []language.Language{
				{Code: "EN", Name: "English"},
				{Code: "FA", Name: "فارسی"},
			}

			expectedResponse = Response{
				Items: []languageResponse{
					{Code: l[0].Code, Name: l[0].Name},
					{Code: l[1].Code, Name: l[1].Name},
				},
			}
		)

		languageRepository.On("Count").Once().Return(uint(len(l)), nil)
		languageRepository.On("GetAll", uint(0), uint(len(l))).Once().Return(l, nil)
		defer languageRepository.AssertExpectations(t)

		response, err := NewUseCase(&languageRepository).Execute()

		assert.NoError(t, err)
		assert.Equal(t, &expectedResponse, response)
	})

	t.Run("counting languages fails", func(t *testing.T) {
		t.Parallel()

		var (
			languageRepository languages.MockLanguagesRepository

			expectedError = errors.New("counting failed")
		)

		languageRepository.On("Count").Once().Return(uint(0), expectedError)
		defer languageRepository.AssertExpectations(t)

		response, err := NewUseCase(&languageRepository).Execute()

		languageRepository.AssertNotCalled(t, "GetAll")
		assert.Nil(t, response)
		assert.ErrorIs(t, err, expectedError)
	})

	t.Run("getting languages fails", func(t *testing.T) {
		t.Parallel()

		var (
			languageRepository languages.MockLanguagesRepository

			expectedError = errors.New("getting failed")
		)

		languageRepository.On("Count").Once().Return(uint(2), nil)
		languageRepository.On("GetAll", uint(0), uint(2)).Once().Return(nil, expectedError)
		defer languageRepository.AssertExpectations(t)

		response, err := NewUseCase(&languageRepository).Execute()

		assert.Nil(t, response)
		assert.ErrorIs(t, err, expectedError)
	})
}
