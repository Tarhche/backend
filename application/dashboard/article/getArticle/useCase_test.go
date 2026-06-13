package getarticle

import (
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/khanzadimahdi/testproject/domain"
	"github.com/khanzadimahdi/testproject/domain/article"
	"github.com/khanzadimahdi/testproject/domain/user"
	"github.com/khanzadimahdi/testproject/infrastructure/repository/mocks/articles"
	"github.com/khanzadimahdi/testproject/infrastructure/repository/mocks/users"
)

func TestUseCase_Execute(t *testing.T) {
	t.Parallel()

	t.Run("getting an article succeeds", func(t *testing.T) {
		t.Parallel()

		var (
			articleRepository articles.MockArticlesRepository
			userRepository    users.MockUsersRepository

			authorUUID = "author-uuid"
			r          = Request{CorrelationUUID: "correlation-uuid", LanguageCode: "EN"}
			a          = article.Article{
				UUID:            "article-uuid",
				CorrelationUUID: r.CorrelationUUID,
				LanguageCode:    r.LanguageCode,
				AuthorUUID:      authorUUID,
			}
			u = user.User{UUID: authorUUID, Name: "author-name", Avatar: "author-avatar", Username: "author-username"}

			expectedResponse = Response{
				CorrelationUUID: r.CorrelationUUID,
				LanguageCode:    r.LanguageCode,
				Author: author{
					UUID:     authorUUID,
					Name:     "author-name",
					Avatar:   "author-avatar",
					Username: "author-username",
				},
				Tags:        []string{},
				PublishedAt: a.PublishedAt.Format(time.RFC3339),
			}
		)

		articleRepository.On("GetByCorrelationUUIDAndLanguage", r.CorrelationUUID, r.LanguageCode).Return(a, nil)
		defer articleRepository.AssertExpectations(t)

		userRepository.On("GetOne", authorUUID).Return(u, nil)
		defer userRepository.AssertExpectations(t)

		response, err := NewUseCase(&articleRepository, &userRepository).Execute(&r)

		assert.NoError(t, err)
		assert.Equal(t, &expectedResponse, response)
	})

	t.Run("getting an article fails", func(t *testing.T) {
		t.Parallel()

		var (
			articleRepository articles.MockArticlesRepository
			userRepository    users.MockUsersRepository

			r             = Request{CorrelationUUID: "correlation-uuid", LanguageCode: "EN"}
			expectedError = errors.New("error")
		)

		articleRepository.On("GetByCorrelationUUIDAndLanguage", r.CorrelationUUID, r.LanguageCode).Return(article.Article{}, expectedError)
		defer articleRepository.AssertExpectations(t)

		response, err := NewUseCase(&articleRepository, &userRepository).Execute(&r)

		userRepository.AssertNotCalled(t, "GetOne")

		assert.ErrorIs(t, err, expectedError)
		assert.Nil(t, response)
	})

	t.Run("getting an author fails", func(t *testing.T) {
		t.Parallel()

		var (
			articleRepository articles.MockArticlesRepository
			userRepository    users.MockUsersRepository

			authorUUID    = "author-uuid"
			expectedError = errors.New("error")
			r             = Request{CorrelationUUID: "correlation-uuid", LanguageCode: "EN"}
			a             = article.Article{
				UUID:            "article-uuid",
				CorrelationUUID: r.CorrelationUUID,
				LanguageCode:    r.LanguageCode,
				AuthorUUID:      authorUUID,
			}
		)

		articleRepository.On("GetByCorrelationUUIDAndLanguage", r.CorrelationUUID, r.LanguageCode).Return(a, nil)
		defer articleRepository.AssertExpectations(t)

		userRepository.On("GetOne", authorUUID).Return(user.User{}, expectedError)
		defer userRepository.AssertExpectations(t)

		response, err := NewUseCase(&articleRepository, &userRepository).Execute(&r)

		assert.ErrorIs(t, err, expectedError)
		assert.Nil(t, response)
	})

	t.Run("missing author is handled gracefully", func(t *testing.T) {
		t.Parallel()

		var (
			articleRepository articles.MockArticlesRepository
			userRepository    users.MockUsersRepository

			authorUUID = "missing-author-uuid"
			r          = Request{CorrelationUUID: "correlation-uuid", LanguageCode: "EN"}
			a          = article.Article{
				UUID:            "article-uuid",
				CorrelationUUID: r.CorrelationUUID,
				LanguageCode:    r.LanguageCode,
				AuthorUUID:      authorUUID,
			}

			expectedResponse = Response{
				CorrelationUUID: r.CorrelationUUID,
				LanguageCode:    r.LanguageCode,
				Author:          author{},
				Tags:            []string{},
				PublishedAt:     a.PublishedAt.Format(time.RFC3339),
			}
		)

		articleRepository.On("GetByCorrelationUUIDAndLanguage", r.CorrelationUUID, r.LanguageCode).Return(a, nil)
		defer articleRepository.AssertExpectations(t)

		userRepository.On("GetOne", authorUUID).Return(user.User{}, domain.ErrNotExists)
		defer userRepository.AssertExpectations(t)

		response, err := NewUseCase(&articleRepository, &userRepository).Execute(&r)

		assert.NoError(t, err)
		assert.Equal(t, &expectedResponse, response)
	})
}
