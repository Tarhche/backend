package getuserarticles

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/khanzadimahdi/testproject/domain/article"
	"github.com/khanzadimahdi/testproject/domain/language"
	"github.com/khanzadimahdi/testproject/domain/user"
	"github.com/khanzadimahdi/testproject/infrastructure/repository/mocks/articles"
	"github.com/khanzadimahdi/testproject/infrastructure/repository/mocks/languages"
	"github.com/khanzadimahdi/testproject/infrastructure/repository/mocks/users"
)

func TestUseCase_Execute(t *testing.T) {
	t.Parallel()

	t.Run("lists what one person wrote", func(t *testing.T) {
		t.Parallel()

		var (
			articleRepository  articles.MockArticlesRepository
			userRepository     users.MockUsersRepository
			languageRepository languages.MockLanguagesRepository

			r                = Request{Page: 1, AuthorUUID: "author-uuid"}
			correlationUUIDs = []string{"correlation-uuid-1", "correlation-uuid-2"}
			a                = []article.Article{
				{UUID: "article-uuid-1", CorrelationUUID: correlationUUIDs[0], AuthorUUID: r.AuthorUUID, LanguageCode: "EN"},
				{UUID: "article-uuid-2", CorrelationUUID: correlationUUIDs[1], AuthorUUID: r.AuthorUUID, LanguageCode: "FA"},
			}
			authors   = []user.User{{UUID: r.AuthorUUID, Name: "author-name"}}
			languages = []language.Language{{Code: "EN"}, {Code: "FA"}}
		)

		// counted and paged as this person's, so a listing of one's own work is
		// not everybody's with the rest taken out afterwards.
		articleRepository.On("CountByCorrelationAndAuthor", mock.Anything, r.AuthorUUID).Once().Return(uint(2), nil)
		articleRepository.On("GetCorrelationUUIDsByAuthor", mock.Anything, r.AuthorUUID, uint(0), uint(limit)).Once().Return(correlationUUIDs, nil)
		articleRepository.On("GetByCorrelationUUIDs", mock.Anything, correlationUUIDs, "").Once().Return(a, nil)
		defer articleRepository.AssertExpectations(t)

		userRepository.On("GetByUUIDs", mock.Anything, []string{r.AuthorUUID, r.AuthorUUID}).Once().Return(authors, nil)
		defer userRepository.AssertExpectations(t)

		languageRepository.On("GetByCodes", mock.Anything, []string{"EN", "FA"}).Once().Return(languages, nil)
		defer languageRepository.AssertExpectations(t)

		response, err := NewUseCase(&articleRepository, &userRepository, &languageRepository).Execute(context.Background(), &r)

		require.NoError(t, err)
		require.NotNil(t, response)
		assert.Len(t, response.Items, len(correlationUUIDs))
	})

	t.Run("somebody who has written nothing has nothing to list", func(t *testing.T) {
		t.Parallel()

		var (
			articleRepository  articles.MockArticlesRepository
			userRepository     users.MockUsersRepository
			languageRepository languages.MockLanguagesRepository

			r = Request{Page: 1, AuthorUUID: "author-uuid"}
		)

		articleRepository.On("CountByCorrelationAndAuthor", mock.Anything, r.AuthorUUID).Once().Return(uint(0), nil)
		articleRepository.On("GetCorrelationUUIDsByAuthor", mock.Anything, r.AuthorUUID, uint(0), uint(limit)).Once().Return([]string{}, nil)
		defer articleRepository.AssertExpectations(t)

		response, err := NewUseCase(&articleRepository, &userRepository, &languageRepository).Execute(context.Background(), &r)

		require.NoError(t, err)
		require.NotNil(t, response)
		assert.Empty(t, response.Items)

		articleRepository.AssertNotCalled(t, "GetByCorrelationUUIDs", mock.Anything, mock.Anything, mock.Anything)
	})

	t.Run("counting fails", func(t *testing.T) {
		t.Parallel()

		var (
			articleRepository  articles.MockArticlesRepository
			userRepository     users.MockUsersRepository
			languageRepository languages.MockLanguagesRepository

			r             = Request{Page: 1, AuthorUUID: "author-uuid"}
			expectedError = errors.New("counting failed")
		)

		articleRepository.On("CountByCorrelationAndAuthor", mock.Anything, r.AuthorUUID).Once().Return(uint(0), expectedError)
		defer articleRepository.AssertExpectations(t)

		response, err := NewUseCase(&articleRepository, &userRepository, &languageRepository).Execute(context.Background(), &r)

		assert.ErrorIs(t, err, expectedError)
		assert.Nil(t, response)
	})
}
