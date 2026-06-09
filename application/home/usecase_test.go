package home

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/khanzadimahdi/testproject/application/element"
	"github.com/khanzadimahdi/testproject/application/language/resolver"
	"github.com/khanzadimahdi/testproject/domain/article"
	domainElement "github.com/khanzadimahdi/testproject/domain/element"
	"github.com/khanzadimahdi/testproject/domain/element/component"
	"github.com/khanzadimahdi/testproject/domain/language"
	"github.com/khanzadimahdi/testproject/domain/user"
	"github.com/khanzadimahdi/testproject/infrastructure/repository/mocks/articles"
	"github.com/khanzadimahdi/testproject/infrastructure/repository/mocks/elements"
	"github.com/khanzadimahdi/testproject/infrastructure/repository/mocks/users"
)

func TestUseCase_Execute(t *testing.T) {
	t.Parallel()

	t.Run("returns articles", func(t *testing.T) {
		t.Parallel()

		var (
			articlesRepository articles.MockArticlesRepository
			elementsRepository elements.MockElementsRepository
			userRepository     users.MockUsersRepository
			languageResolver   resolver.MockResolver
			mockComponent      component.MockComponent

			a = []article.Article{
				{UUID: "test-article-1", AuthorUUID: "author-uuid-1"},
				{UUID: "test-article-2", AuthorUUID: "author-uuid-2"},
				{UUID: "test-article-3", AuthorUUID: "author-uuid-1"},
			}

			va = []article.Article{
				{UUID: "test-article-1", AuthorUUID: "author-uuid-1"},
				{UUID: "test-article-2", AuthorUUID: "author-uuid-2"},
			}

			u = []user.User{
				{UUID: "author-uuid-1", Name: "author-name-1", Avatar: "author-avatar-1"},
				{UUID: "author-uuid-2", Name: "author-name-2", Avatar: "author-avatar-2"},
			}

			i = []component.Item{
				{ContentUUID: va[0].UUID},
				{ContentUUID: va[1].UUID},
				{ContentUUID: "not-exist-article-uuid"},
			}

			articleUUIDs = []string{
				i[0].ContentUUID,
				i[1].ContentUUID,
				i[2].ContentUUID,
			}

			homeAuthorUUIDs    = []string{"author-uuid-1", "author-uuid-2", "author-uuid-1", "author-uuid-1", "author-uuid-2", "author-uuid-1"}
			elementAuthorUUIDs = []string{"author-uuid-1", "author-uuid-2"}
		)

		articlesRepository.On("GetMostViewed", "EN", uint(4)).Once().Return(a, nil)
		articlesRepository.On("GetAllPublished", "EN", uint(0), uint(3)).Once().Return(a, nil)
		articlesRepository.On("GetByCorrelationUUIDs", articleUUIDs, "EN").Once().Return(va, nil)
		defer articlesRepository.AssertExpectations(t)

		userRepository.On("GetByUUIDs", homeAuthorUUIDs).Once().Return(u, nil)
		userRepository.On("GetByUUIDs", elementAuthorUUIDs).Once().Return(u, nil)
		defer userRepository.AssertExpectations(t)

		mockComponent.On("Items").Once().Return(i)
		mockComponent.On("Type").Return(component.ComponentTypeMock)
		defer mockComponent.AssertExpectations(t)

		v := []domainElement.Element{
			{Body: &mockComponent},
		}

		elementsRepository.On("GetByVenues", []string{"home"}).Once().Return(v, nil)
		defer elementsRepository.AssertExpectations(t)

		languageResolver.On("DefaultCode").Once().Return("EN", nil)
		languageResolver.On("Resolve", "EN").Once().Return(language.Language{Code: "EN"}, nil)
		defer languageResolver.AssertExpectations(t)

		elementRetriever := element.NewRetriever(&articlesRepository, &elementsRepository, &userRepository)
		usecase := NewUseCase(&articlesRepository, &userRepository, elementRetriever, &languageResolver)
		response, err := usecase.Execute(&Request{})

		assert.NotNil(t, response, "unexpected response")
		assert.NoError(t, err)
	})

	t.Run("error on getting most viewed articles", func(t *testing.T) {
		t.Parallel()

		var (
			articlesRepository articles.MockArticlesRepository
			elementsRepository elements.MockElementsRepository
			userRepository     users.MockUsersRepository
			languageResolver   resolver.MockResolver

			expectedErr = errors.New("some error")
		)

		articlesRepository.On("GetMostViewed", "EN", uint(4)).Once().Return(nil, expectedErr)
		defer articlesRepository.AssertExpectations(t)

		languageResolver.On("DefaultCode").Once().Return("EN", nil)
		languageResolver.On("Resolve", "EN").Once().Return(language.Language{Code: "EN"}, nil)
		defer languageResolver.AssertExpectations(t)

		elementRetriever := element.NewRetriever(&articlesRepository, &elementsRepository, &userRepository)
		usecase := NewUseCase(&articlesRepository, &userRepository, elementRetriever, &languageResolver)
		response, err := usecase.Execute(&Request{})

		articlesRepository.AssertNotCalled(t, "GetAllPublished")
		articlesRepository.AssertNotCalled(t, "GetByCorrelationUUIDs")
		userRepository.AssertNotCalled(t, "GetByUUIDs")
		elementsRepository.AssertNotCalled(t, "GetByVenues")

		assert.Nil(t, response, "unexpected response")
		assert.ErrorIs(t, err, expectedErr)
	})

	t.Run("error on getting all published articles", func(t *testing.T) {
		t.Parallel()

		var (
			articlesRepository articles.MockArticlesRepository
			elementsRepository elements.MockElementsRepository
			userRepository     users.MockUsersRepository
			languageResolver   resolver.MockResolver

			a = []article.Article{
				{UUID: "test-article-1"},
				{UUID: "test-article-2"},
				{UUID: "test-article-3"},
			}

			expectedErr = errors.New("some error")
		)

		articlesRepository.On("GetMostViewed", "EN", uint(4)).Once().Return(a, nil)
		articlesRepository.On("GetAllPublished", "EN", uint(0), uint(3)).Return(nil, expectedErr)
		defer articlesRepository.AssertExpectations(t)

		languageResolver.On("DefaultCode").Once().Return("EN", nil)
		languageResolver.On("Resolve", "EN").Once().Return(language.Language{Code: "EN"}, nil)
		defer languageResolver.AssertExpectations(t)

		elementRetriever := element.NewRetriever(&articlesRepository, &elementsRepository, &userRepository)
		usecase := NewUseCase(&articlesRepository, &userRepository, elementRetriever, &languageResolver)
		response, err := usecase.Execute(&Request{})

		elementsRepository.AssertNotCalled(t, "GetByVenues")
		articlesRepository.AssertNotCalled(t, "GetByCorrelationUUIDs")
		userRepository.AssertNotCalled(t, "GetByUUIDs")

		assert.Nil(t, response, "unexpected response")
		assert.ErrorIs(t, err, expectedErr)
	})

	t.Run("error on getting authors", func(t *testing.T) {
		t.Parallel()

		var (
			articlesRepository articles.MockArticlesRepository
			elementsRepository elements.MockElementsRepository
			userRepository     users.MockUsersRepository
			languageResolver   resolver.MockResolver

			a = []article.Article{
				{UUID: "test-article-1", AuthorUUID: "author-uuid-1"},
			}

			expectedErr = errors.New("some error")
		)

		articlesRepository.On("GetMostViewed", "EN", uint(4)).Once().Return(a, nil)
		articlesRepository.On("GetAllPublished", "EN", uint(0), uint(3)).Once().Return(a, nil)
		defer articlesRepository.AssertExpectations(t)

		userRepository.On("GetByUUIDs", []string{"author-uuid-1", "author-uuid-1"}).Once().Return(nil, expectedErr)
		defer userRepository.AssertExpectations(t)

		languageResolver.On("DefaultCode").Once().Return("EN", nil)
		languageResolver.On("Resolve", "EN").Once().Return(language.Language{Code: "EN"}, nil)
		defer languageResolver.AssertExpectations(t)

		elementRetriever := element.NewRetriever(&articlesRepository, &elementsRepository, &userRepository)
		usecase := NewUseCase(&articlesRepository, &userRepository, elementRetriever, &languageResolver)
		response, err := usecase.Execute(&Request{})

		articlesRepository.AssertNotCalled(t, "GetByCorrelationUUIDs")
		elementsRepository.AssertNotCalled(t, "GetByVenues")

		assert.Nil(t, response, "unexpected response")
		assert.ErrorIs(t, err, expectedErr)
	})

	t.Run("error on getting elements", func(t *testing.T) {
		t.Parallel()

		var (
			articlesRepository articles.MockArticlesRepository
			elementsRepository elements.MockElementsRepository
			userRepository     users.MockUsersRepository
			languageResolver   resolver.MockResolver

			a = []article.Article{
				{UUID: "test-article-1", AuthorUUID: "author-uuid-1"},
			}
			u = []user.User{
				{UUID: "author-uuid-1", Name: "author-name-1", Avatar: "author-avatar-1"},
			}

			expectedErr = errors.New("some error")
		)

		articlesRepository.On("GetMostViewed", "EN", uint(4)).Once().Return(a, nil)
		articlesRepository.On("GetAllPublished", "EN", uint(0), uint(3)).Return(a, nil)
		defer articlesRepository.AssertExpectations(t)

		userRepository.On("GetByUUIDs", []string{"author-uuid-1", "author-uuid-1"}).Once().Return(u, nil)
		defer userRepository.AssertExpectations(t)

		elementsRepository.On("GetByVenues", []string{"home"}).Once().Return(nil, expectedErr)
		defer elementsRepository.AssertExpectations(t)

		languageResolver.On("DefaultCode").Once().Return("EN", nil)
		languageResolver.On("Resolve", "EN").Once().Return(language.Language{Code: "EN"}, nil)
		defer languageResolver.AssertExpectations(t)

		elementRetriever := element.NewRetriever(&articlesRepository, &elementsRepository, &userRepository)
		usecase := NewUseCase(&articlesRepository, &userRepository, elementRetriever, &languageResolver)
		response, err := usecase.Execute(&Request{})

		articlesRepository.AssertNotCalled(t, "GetByCorrelationUUIDs")

		assert.Nil(t, response, "unexpected response")
		assert.ErrorIs(t, err, expectedErr)
	})

	t.Run("error on getting element articles", func(t *testing.T) {
		t.Parallel()

		var (
			articlesRepository articles.MockArticlesRepository
			elementsRepository elements.MockElementsRepository
			userRepository     users.MockUsersRepository
			languageResolver   resolver.MockResolver
			mockComponent      component.MockComponent

			a = []article.Article{
				{UUID: "test-article-1", AuthorUUID: "author-uuid-1"},
				{UUID: "test-article-2", AuthorUUID: "author-uuid-2"},
				{UUID: "test-article-3", AuthorUUID: "author-uuid-1"},
			}

			va = []article.Article{
				{UUID: "test-article-1"},
				{UUID: "test-article-2"},
			}

			u = []user.User{
				{UUID: "author-uuid-1", Name: "author-name-1", Avatar: "author-avatar-1"},
				{UUID: "author-uuid-2", Name: "author-name-2", Avatar: "author-avatar-2"},
			}

			i = []component.Item{
				{ContentUUID: va[0].UUID},
				{ContentUUID: va[1].UUID},
				{ContentUUID: "not-exist-article-uuid"},
			}

			articleUUIDs = []string{
				i[0].ContentUUID,
				i[1].ContentUUID,
				i[2].ContentUUID,
			}

			homeAuthorUUIDs = []string{"author-uuid-1", "author-uuid-2", "author-uuid-1", "author-uuid-1", "author-uuid-2", "author-uuid-1"}

			expectedErr = errors.New("some error")
		)

		articlesRepository.On("GetMostViewed", "EN", uint(4)).Once().Return(a, nil)
		articlesRepository.On("GetAllPublished", "EN", uint(0), uint(3)).Once().Return(a, nil)
		articlesRepository.On("GetByCorrelationUUIDs", articleUUIDs, "EN").Once().Return(nil, expectedErr)
		defer articlesRepository.AssertExpectations(t)

		userRepository.On("GetByUUIDs", homeAuthorUUIDs).Once().Return(u, nil)
		defer userRepository.AssertExpectations(t)

		mockComponent.On("Items").Once().Return(i)
		defer mockComponent.AssertExpectations(t)

		v := []domainElement.Element{
			{Body: &mockComponent},
		}

		elementsRepository.On("GetByVenues", []string{"home"}).Once().Return(v, nil)
		defer elementsRepository.AssertExpectations(t)

		languageResolver.On("DefaultCode").Once().Return("EN", nil)
		languageResolver.On("Resolve", "EN").Once().Return(language.Language{Code: "EN"}, nil)
		defer languageResolver.AssertExpectations(t)

		elementRetriever := element.NewRetriever(&articlesRepository, &elementsRepository, &userRepository)
		usecase := NewUseCase(&articlesRepository, &userRepository, elementRetriever, &languageResolver)
		response, err := usecase.Execute(&Request{})

		assert.Nil(t, response, "unexpected response")
		assert.ErrorIs(t, err, expectedErr)
	})
}
