package deletearticle

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"

	"github.com/khanzadimahdi/testproject/infrastructure/repository/mocks/articles"
)

func TestUseCase_Execute(t *testing.T) {
	t.Parallel()

	t.Run("deleting an article succeeds", func(t *testing.T) {
		t.Parallel()

		var (
			articleRepository articles.MockArticlesRepository

			r = Request{CorrelationUUID: "correlation-uuid", LanguageCode: "EN"}
		)

		articleRepository.On("DeleteByCorrelationUUIDAndLanguage", mock.Anything, r.CorrelationUUID, r.LanguageCode).Return(nil)
		defer articleRepository.AssertExpectations(t)

		err := NewUseCase(&articleRepository).Execute(context.Background(), &r)

		assert.NoError(t, err)
	})

	t.Run("deleting an article fails", func(t *testing.T) {
		t.Parallel()

		var (
			articleRepository articles.MockArticlesRepository

			r             = Request{CorrelationUUID: "correlation-uuid", LanguageCode: "EN"}
			expectedError = errors.New("article deletion failed")
		)

		articleRepository.On("DeleteByCorrelationUUIDAndLanguage", mock.Anything, r.CorrelationUUID, r.LanguageCode).Return(expectedError)
		defer articleRepository.AssertExpectations(t)

		err := NewUseCase(&articleRepository).Execute(context.Background(), &r)

		assert.ErrorIs(t, err, expectedError)
	})
}
