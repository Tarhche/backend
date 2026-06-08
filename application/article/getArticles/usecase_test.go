package getarticles

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/khanzadimahdi/testproject/application/language/resolver"
	"github.com/khanzadimahdi/testproject/domain/article"
	"github.com/khanzadimahdi/testproject/domain/language"
	"github.com/khanzadimahdi/testproject/domain/user"
	"github.com/khanzadimahdi/testproject/infrastructure/repository/mocks/articles"
	"github.com/khanzadimahdi/testproject/infrastructure/repository/mocks/users"
)

func TestUseCase_Execute(t *testing.T) {
	t.Run("returns articles", func(t *testing.T) {
		var (
			articlesRepository articles.MockArticlesRepository
			userRepository     users.MockUsersRepository
			languageResolver   resolver.MockResolver

			a = []article.Article{
				{UUID: "test-article-1", AuthorUUID: "author-uuid-1"},
				{UUID: "test-article-2", AuthorUUID: "author-uuid-2"},
				{UUID: "test-article-3", AuthorUUID: "author-uuid-1"},
			}
			u = []user.User{
				{UUID: "author-uuid-1", Name: "author-name-1", Avatar: "author-avatar-1"},
				{UUID: "author-uuid-2", Name: "author-name-2", Avatar: "author-avatar-2"},
			}
		)

		articlesRepository.On("CountPublished", "EN").Once().Return(uint(1), nil)
		articlesRepository.On("GetAllPublished", "EN", uint(0), uint(10)).Once().Return(a, nil)
		defer articlesRepository.AssertExpectations(t)

		userRepository.On("GetByUUIDs", []string{"author-uuid-1", "author-uuid-2", "author-uuid-1"}).Once().Return(u, nil)
		defer userRepository.AssertExpectations(t)

		articlesRepository.On("GetPublishedLanguages", "").Return([]language.Language{}, nil)

		languageResolver.On("DefaultCode").Once().Return("EN", nil)
		languageResolver.On("Resolve", "EN").Once().Return(language.Language{Code: "EN"}, nil)
		defer languageResolver.AssertExpectations(t)

		usecase := NewUseCase(&articlesRepository, &userRepository, &languageResolver)
		request := Request{Page: 1}
		response, err := usecase.Execute(&request)

		assert.NotNil(t, response, "unexpected response")
		assert.NoError(t, err)
	})

	t.Run("returns an error on counting items", func(t *testing.T) {
		var (
			articlesRepository articles.MockArticlesRepository
			userRepository     users.MockUsersRepository
			languageResolver   resolver.MockResolver
			expectedErr        = errors.New("test error")
		)

		articlesRepository.On("CountPublished", "EN").Once().Return(uint(1), expectedErr)
		defer articlesRepository.AssertExpectations(t)

		languageResolver.On("DefaultCode").Once().Return("EN", nil)
		languageResolver.On("Resolve", "EN").Once().Return(language.Language{Code: "EN"}, nil)
		defer languageResolver.AssertExpectations(t)

		usecase := NewUseCase(&articlesRepository, &userRepository, &languageResolver)
		request := Request{Page: 1}
		response, err := usecase.Execute(&request)

		articlesRepository.AssertNotCalled(t, "GetAllPublished")
		userRepository.AssertNotCalled(t, "GetByUUIDs")

		assert.Nil(t, response, "unexpected response")
		assert.ErrorIs(t, err, expectedErr)
	})

	t.Run("returns an error on getting items", func(t *testing.T) {
		var (
			articlesRepository articles.MockArticlesRepository
			userRepository     users.MockUsersRepository
			languageResolver   resolver.MockResolver
			expectedErr        = errors.New("test error")
		)

		articlesRepository.On("CountPublished", "EN").Once().Return(uint(1), nil)
		articlesRepository.On("GetAllPublished", "EN", uint(0), uint(10)).Once().Return(nil, expectedErr)
		defer articlesRepository.AssertExpectations(t)

		languageResolver.On("DefaultCode").Once().Return("EN", nil)
		languageResolver.On("Resolve", "EN").Once().Return(language.Language{Code: "EN"}, nil)
		defer languageResolver.AssertExpectations(t)

		usecase := NewUseCase(&articlesRepository, &userRepository, &languageResolver)
		request := Request{Page: 1}
		response, err := usecase.Execute(&request)

		userRepository.AssertNotCalled(t, "GetByUUIDs")

		assert.Nil(t, response, "unexpected response")
		assert.ErrorIs(t, err, expectedErr)
	})

	t.Run("returns an error on getting authors", func(t *testing.T) {
		var (
			articlesRepository articles.MockArticlesRepository
			userRepository     users.MockUsersRepository
			languageResolver   resolver.MockResolver
			expectedErr        = errors.New("test error")

			a = []article.Article{
				{UUID: "test-article-1", AuthorUUID: "author-uuid-1"},
			}
		)

		articlesRepository.On("CountPublished", "EN").Once().Return(uint(1), nil)
		articlesRepository.On("GetAllPublished", "EN", uint(0), uint(10)).Once().Return(a, nil)
		defer articlesRepository.AssertExpectations(t)

		userRepository.On("GetByUUIDs", []string{"author-uuid-1"}).Once().Return(nil, expectedErr)
		defer userRepository.AssertExpectations(t)

		languageResolver.On("DefaultCode").Once().Return("EN", nil)
		languageResolver.On("Resolve", "EN").Once().Return(language.Language{Code: "EN"}, nil)
		defer languageResolver.AssertExpectations(t)

		usecase := NewUseCase(&articlesRepository, &userRepository, &languageResolver)
		request := Request{Page: 1}
		response, err := usecase.Execute(&request)

		assert.Nil(t, response, "unexpected response")
		assert.ErrorIs(t, err, expectedErr)
	})
}
