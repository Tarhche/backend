package deleteuserarticle

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"

	"github.com/khanzadimahdi/testproject/domain"
	"github.com/khanzadimahdi/testproject/domain/article"
	"github.com/khanzadimahdi/testproject/infrastructure/repository/mocks/articles"
)

func TestUseCase_Execute(t *testing.T) {
	t.Parallel()

	t.Run("deletes an article the person wrote", func(t *testing.T) {
		t.Parallel()

		var (
			articleRepository articles.MockArticlesRepository

			r = Request{CorrelationUUID: "correlation-uuid", LanguageCode: "EN", AuthorUUID: "author-uuid"}
			a = article.Article{
				UUID:            "article-uuid",
				CorrelationUUID: r.CorrelationUUID,
				LanguageCode:    r.LanguageCode,
				AuthorUUID:      r.AuthorUUID,
			}
		)

		articleRepository.On("GetByCorrelationUUIDAndLanguageAndAuthor", mock.Anything, r.CorrelationUUID, r.LanguageCode, r.AuthorUUID).Once().Return(a, nil)
		articleRepository.On("DeleteByCorrelationUUIDAndLanguageAndAuthor", mock.Anything, r.CorrelationUUID, r.LanguageCode, r.AuthorUUID).Once().Return(nil)
		defer articleRepository.AssertExpectations(t)

		assert.NoError(t, NewUseCase(&articleRepository).Execute(context.Background(), &r))
	})

	t.Run("an article somebody else wrote is not there to delete", func(t *testing.T) {
		t.Parallel()

		var (
			articleRepository articles.MockArticlesRepository

			r = Request{CorrelationUUID: "correlation-uuid", LanguageCode: "EN", AuthorUUID: "author-uuid"}
		)

		// asked for as this person's, an article that is not theirs is not
		// found — the same answer as one that does not exist.
		articleRepository.On("GetByCorrelationUUIDAndLanguageAndAuthor", mock.Anything, r.CorrelationUUID, r.LanguageCode, r.AuthorUUID).
			Once().Return(article.Article{}, domain.ErrNotExists)
		defer articleRepository.AssertExpectations(t)

		assert.ErrorIs(t, NewUseCase(&articleRepository).Execute(context.Background(), &r), domain.ErrNotExists)

		articleRepository.AssertNotCalled(t, "DeleteByCorrelationUUIDAndLanguageAndAuthor", mock.Anything, mock.Anything, mock.Anything, mock.Anything)
	})

	t.Run("deleting fails", func(t *testing.T) {
		t.Parallel()

		var (
			articleRepository articles.MockArticlesRepository

			r             = Request{CorrelationUUID: "correlation-uuid", LanguageCode: "EN", AuthorUUID: "author-uuid"}
			a             = article.Article{CorrelationUUID: r.CorrelationUUID, LanguageCode: r.LanguageCode, AuthorUUID: r.AuthorUUID}
			expectedError = errors.New("article deletion failed")
		)

		articleRepository.On("GetByCorrelationUUIDAndLanguageAndAuthor", mock.Anything, r.CorrelationUUID, r.LanguageCode, r.AuthorUUID).Once().Return(a, nil)
		articleRepository.On("DeleteByCorrelationUUIDAndLanguageAndAuthor", mock.Anything, r.CorrelationUUID, r.LanguageCode, r.AuthorUUID).Once().Return(expectedError)
		defer articleRepository.AssertExpectations(t)

		assert.ErrorIs(t, NewUseCase(&articleRepository).Execute(context.Background(), &r), expectedError)
	})
}
