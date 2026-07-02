package deletelanguage

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"

	"github.com/khanzadimahdi/testproject/infrastructure/repository/mocks/languages"
)

func TestUseCase_Execute(t *testing.T) {
	t.Parallel()

	t.Run("deletes a language", func(t *testing.T) {
		t.Parallel()

		var (
			languageRepository languages.MockLanguagesRepository

			r = Request{Code: "EN"}
		)

		languageRepository.On("Delete", mock.Anything, r.Code).Once().Return(nil)
		defer languageRepository.AssertExpectations(t)

		err := NewUseCase(&languageRepository).Execute(context.Background(), &r)

		assert.NoError(t, err)
	})

	t.Run("deleting the language fails", func(t *testing.T) {
		t.Parallel()

		var (
			languageRepository languages.MockLanguagesRepository

			r             = Request{Code: "EN"}
			expectedError = errors.New("language deletion failed")
		)

		languageRepository.On("Delete", mock.Anything, r.Code).Once().Return(expectedError)
		defer languageRepository.AssertExpectations(t)

		err := NewUseCase(&languageRepository).Execute(context.Background(), &r)

		assert.ErrorIs(t, err, expectedError)
	})
}
