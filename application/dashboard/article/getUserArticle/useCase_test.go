package getuserarticle

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/khanzadimahdi/testproject/domain"
	"github.com/khanzadimahdi/testproject/domain/article"
	"github.com/khanzadimahdi/testproject/domain/user"
	"github.com/khanzadimahdi/testproject/infrastructure/repository/mocks/articles"
	"github.com/khanzadimahdi/testproject/infrastructure/repository/mocks/users"
)

func TestUseCase_Execute(t *testing.T) {
	t.Parallel()

	t.Run("shows an article the person wrote", func(t *testing.T) {
		t.Parallel()

		var (
			articleRepository articles.MockArticlesRepository
			userRepository    users.MockUsersRepository

			r = Request{CorrelationUUID: "correlation-uuid", LanguageCode: "EN", AuthorUUID: "author-uuid"}
			a = article.Article{
				UUID:            "article-uuid",
				CorrelationUUID: r.CorrelationUUID,
				LanguageCode:    r.LanguageCode,
				AuthorUUID:      r.AuthorUUID,
			}
			u = user.User{UUID: r.AuthorUUID, Name: "author-name", Avatar: "author-avatar", Username: "author-username"}

			expectedResponse = Response{
				CorrelationUUID: r.CorrelationUUID,
				LanguageCode:    r.LanguageCode,
				Author: author{
					UUID:     r.AuthorUUID,
					Name:     "author-name",
					Avatar:   "author-avatar",
					Username: "author-username",
				},
				Tags:        []string{},
				PublishedAt: a.PublishedAt.Format(time.RFC3339),
			}
		)

		articleRepository.On("GetByCorrelationUUIDAndLanguageAndAuthor", mock.Anything, r.CorrelationUUID, r.LanguageCode, r.AuthorUUID).Once().Return(a, nil)
		defer articleRepository.AssertExpectations(t)

		userRepository.On("GetOne", mock.Anything, r.AuthorUUID).Once().Return(u, nil)
		defer userRepository.AssertExpectations(t)

		response, err := NewUseCase(&articleRepository, &userRepository).Execute(context.Background(), &r)

		require.NoError(t, err)
		assert.Equal(t, &expectedResponse, response)
	})

	t.Run("an article somebody else wrote is not there to show", func(t *testing.T) {
		t.Parallel()

		var (
			articleRepository articles.MockArticlesRepository
			userRepository    users.MockUsersRepository

			r = Request{CorrelationUUID: "correlation-uuid", LanguageCode: "EN", AuthorUUID: "author-uuid"}
		)

		// asked for as this person's, an article that is not theirs is not
		// found — the same answer as one that does not exist.
		articleRepository.On("GetByCorrelationUUIDAndLanguageAndAuthor", mock.Anything, r.CorrelationUUID, r.LanguageCode, r.AuthorUUID).
			Once().Return(article.Article{}, domain.ErrNotExists)
		defer articleRepository.AssertExpectations(t)

		response, err := NewUseCase(&articleRepository, &userRepository).Execute(context.Background(), &r)

		assert.ErrorIs(t, err, domain.ErrNotExists)
		assert.Nil(t, response)

		userRepository.AssertNotCalled(t, "GetOne", mock.Anything, mock.Anything)
	})
}
