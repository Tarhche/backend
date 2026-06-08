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

	t.Run("returns a paginated list of languages", func(t *testing.T) {
		t.Parallel()

		var (
			languageRepository languages.MockLanguagesRepository

			l = []language.Language{
				{Code: "EN", Name: "English"},
				{Code: "FA", Name: "فارسی"},
			}

			request = Request{Page: 1}

			expectedResponse = Response{
				Items: []languageResponse{
					{Code: l[0].Code, Name: l[0].Name},
					{Code: l[1].Code, Name: l[1].Name},
				},
				Pagination: pagination{TotalPages: 1, CurrentPage: 1},
			}
		)

		languageRepository.On("Count").Once().Return(uint(len(l)), nil)
		languageRepository.On("GetAll", uint(0), uint(limit)).Once().Return(l, nil)
		defer languageRepository.AssertExpectations(t)

		response, err := NewUseCase(&languageRepository).Execute(&request)

		assert.NoError(t, err)
		assert.Equal(t, &expectedResponse, response)
	})

	t.Run("counting languages fails", func(t *testing.T) {
		t.Parallel()

		var (
			languageRepository languages.MockLanguagesRepository

			request       = Request{Page: 1}
			expectedError = errors.New("counting failed")
		)

		languageRepository.On("Count").Once().Return(uint(0), expectedError)
		defer languageRepository.AssertExpectations(t)

		response, err := NewUseCase(&languageRepository).Execute(&request)

		languageRepository.AssertNotCalled(t, "GetAll")
		assert.Nil(t, response)
		assert.ErrorIs(t, err, expectedError)
	})

	t.Run("getting languages fails", func(t *testing.T) {
		t.Parallel()

		var (
			languageRepository languages.MockLanguagesRepository

			request       = Request{Page: 1}
			expectedError = errors.New("getting failed")
		)

		languageRepository.On("Count").Once().Return(uint(2), nil)
		languageRepository.On("GetAll", uint(0), uint(limit)).Once().Return(nil, expectedError)
		defer languageRepository.AssertExpectations(t)

		response, err := NewUseCase(&languageRepository).Execute(&request)

		assert.Nil(t, response)
		assert.ErrorIs(t, err, expectedError)
	})
}
