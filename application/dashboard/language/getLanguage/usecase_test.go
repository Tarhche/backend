package getlanguage

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"

	"github.com/khanzadimahdi/testproject/domain"
	"github.com/khanzadimahdi/testproject/domain/language"
	"github.com/khanzadimahdi/testproject/infrastructure/repository/mocks/languages"
)

func TestUseCase_Execute(t *testing.T) {
	t.Parallel()

	t.Run("returns a language", func(t *testing.T) {
		t.Parallel()

		var (
			languageRepository languages.MockLanguagesRepository

			l = language.Language{Code: "EN", Name: "English"}

			expectedResponse = Response{Code: l.Code, Name: l.Name}
		)

		languageRepository.On("GetOne", mock.Anything, l.Code).Once().Return(l, nil)
		defer languageRepository.AssertExpectations(t)

		response, err := NewUseCase(&languageRepository).Execute(context.Background(), l.Code)

		assert.NoError(t, err)
		assert.Equal(t, &expectedResponse, response)
	})

	t.Run("language does not exist", func(t *testing.T) {
		t.Parallel()

		var (
			languageRepository languages.MockLanguagesRepository
		)

		languageRepository.On("GetOne", mock.Anything, "DE").Once().Return(language.Language{}, domain.ErrNotExists)
		defer languageRepository.AssertExpectations(t)

		response, err := NewUseCase(&languageRepository).Execute(context.Background(), "DE")

		assert.Nil(t, response)
		assert.ErrorIs(t, err, domain.ErrNotExists)
	})
}
