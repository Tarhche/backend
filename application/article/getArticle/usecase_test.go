package getarticle

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/khanzadimahdi/testproject/application/element"
	"github.com/khanzadimahdi/testproject/application/language/resolver"
	"github.com/khanzadimahdi/testproject/domain"
	"github.com/khanzadimahdi/testproject/domain/article"
	domainElement "github.com/khanzadimahdi/testproject/domain/element"
	"github.com/khanzadimahdi/testproject/domain/element/component"
	"github.com/khanzadimahdi/testproject/domain/language"
	"github.com/khanzadimahdi/testproject/domain/user"
	"github.com/khanzadimahdi/testproject/infrastructure/repository/mocks/articles"
	"github.com/khanzadimahdi/testproject/infrastructure/repository/mocks/elements"
	"github.com/khanzadimahdi/testproject/infrastructure/repository/mocks/users"
	"github.com/khanzadimahdi/testproject/infrastructure/validator"
)

func TestUseCase_Execute(t *testing.T) {
	t.Run("returns an article", func(t *testing.T) {
		var (
			articlesRepository articles.MockArticlesRepository
			elementsRepository elements.MockElementsRepository
			userRepository     users.MockUsersRepository
			languageResolver   resolver.MockResolver
			requestValidator   validator.MockValidator
			mockComponent      component.MockComponent

			correlationUUID string   = "test-correlation-uuid"
			articleUUID     string   = "test-uuid"
			authorUUID      string   = "author-uuid"
			venues          []string = []string{"articles/*", fmt.Sprintf("articles/%s", correlationUUID)}
			increaseView    uint     = 1

			a = article.Article{
				UUID:            articleUUID,
				AuthorUUID:      authorUUID,
				LanguageCode:    "EN",
				CorrelationUUID: correlationUUID,
			}
			au = user.User{UUID: authorUUID, Name: "author-name", Avatar: "author-avatar"}
			va = []article.Article{
				{UUID: "test-article-1", AuthorUUID: "author-uuid-1"},
				{UUID: "test-article-2", AuthorUUID: "author-uuid-2"},
			}
			elementUsers = []user.User{
				{UUID: "author-uuid-1", Name: "author-name-1", Avatar: "author-avatar-1"},
				{UUID: "author-uuid-2", Name: "author-name-2", Avatar: "author-avatar-2"},
			}
			i = []component.Item{
				{ContentUUID: va[0].UUID},
				{ContentUUID: va[1].UUID},
				{ContentUUID: "not-exist-article-uuid"},
			}
			u = []string{
				i[0].ContentUUID,
				i[1].ContentUUID,
				i[2].ContentUUID,
			}

			request = Request{CorrelationUUID: correlationUUID}
		)

		requestValidator.On("Validate", &request).Once().Return(nil)
		defer requestValidator.AssertExpectations(t)

		languageResolver.On("DefaultCode").Once().Return("EN", nil)
		languageResolver.On("Resolve", "EN").Once().Return(language.Language{Code: "EN"}, nil)
		defer languageResolver.AssertExpectations(t)

		articlesRepository.On("GetOnePublished", correlationUUID, "EN").Once().Return(a, nil)
		articlesRepository.On("GetPublishedLanguages", correlationUUID).Once().Return([]language.Language{{Code: "EN"}}, nil)
		articlesRepository.On("IncreaseView", articleUUID, increaseView).Once().Return(nil)
		articlesRepository.On("GetByCorrelationUUIDs", u, "EN").Once().Return(va, nil)
		defer articlesRepository.AssertExpectations(t)

		userRepository.On("GetOne", authorUUID).Once().Return(au, nil)
		userRepository.On("GetByUUIDs", []string{"author-uuid-1", "author-uuid-2"}).Once().Return(elementUsers, nil)
		defer userRepository.AssertExpectations(t)

		mockComponent.On("Items").Once().Return(i)
		mockComponent.On("Type").Return(component.ComponentTypeMock)
		defer mockComponent.AssertExpectations(t)

		v := []domainElement.Element{
			{Body: &mockComponent},
		}

		elementsRepository.On("GetByVenues", venues).Once().Return(v, nil)
		defer elementsRepository.AssertExpectations(t)

		elementRetriever := element.NewRetriever(&articlesRepository, &elementsRepository, &userRepository)
		response, err := NewUseCase(&articlesRepository, &userRepository, &languageResolver, elementRetriever, &requestValidator).Execute(&request)

		assert.NotNil(t, response, "unexpected response")
		assert.NoError(t, err, "unexpected error")
	})

	t.Run("error on getting article", func(t *testing.T) {
		var (
			articlesRepository articles.MockArticlesRepository
			elementsRepository elements.MockElementsRepository
			userRepository     users.MockUsersRepository
			languageResolver   resolver.MockResolver
			requestValidator   validator.MockValidator

			correlationUUID string = "test-correlation-uuid"
			expectedErr            = domain.ErrNotExists

			request = Request{CorrelationUUID: correlationUUID}
		)

		requestValidator.On("Validate", &request).Once().Return(nil)
		defer requestValidator.AssertExpectations(t)

		languageResolver.On("DefaultCode").Once().Return("EN", nil)
		languageResolver.On("Resolve", "EN").Once().Return(language.Language{Code: "EN"}, nil)
		defer languageResolver.AssertExpectations(t)

		articlesRepository.On("GetOnePublished", correlationUUID, "EN").Once().Return(article.Article{}, expectedErr)
		defer articlesRepository.AssertExpectations(t)

		elementRetriever := element.NewRetriever(&articlesRepository, &elementsRepository, &userRepository)
		usecase := NewUseCase(&articlesRepository, &userRepository, &languageResolver, elementRetriever, &requestValidator)
		response, err := usecase.Execute(&request)

		userRepository.AssertNotCalled(t, "GetOne")
		articlesRepository.AssertNotCalled(t, "GetPublishedLanguages")
		elementsRepository.AssertNotCalled(t, "GetByVenues")
		articlesRepository.AssertNotCalled(t, "GetByCorrelationUUIDs")
		articlesRepository.AssertNotCalled(t, "IncreaseView")

		assert.Nil(t, response, "unexpected response")
		assert.ErrorIs(t, err, expectedErr)
	})

	t.Run("missing author is handled gracefully", func(t *testing.T) {
		var (
			articlesRepository articles.MockArticlesRepository
			elementsRepository elements.MockElementsRepository
			userRepository     users.MockUsersRepository
			languageResolver   resolver.MockResolver
			requestValidator   validator.MockValidator
			mockComponent      component.MockComponent

			correlationUUID string   = "test-correlation-uuid"
			articleUUID     string   = "test-uuid"
			authorUUID      string   = "missing-author-uuid"
			venues          []string = []string{"articles/*", fmt.Sprintf("articles/%s", correlationUUID)}
			increaseView    uint     = 1

			a = article.Article{
				UUID:            articleUUID,
				AuthorUUID:      authorUUID,
				LanguageCode:    "EN",
				CorrelationUUID: correlationUUID,
			}

			request = Request{CorrelationUUID: correlationUUID}
		)

		requestValidator.On("Validate", &request).Once().Return(nil)
		defer requestValidator.AssertExpectations(t)

		languageResolver.On("DefaultCode").Once().Return("EN", nil)
		languageResolver.On("Resolve", "EN").Once().Return(language.Language{Code: "EN"}, nil)
		defer languageResolver.AssertExpectations(t)

		articlesRepository.On("GetOnePublished", correlationUUID, "EN").Once().Return(a, nil)
		articlesRepository.On("GetPublishedLanguages", correlationUUID).Once().Return([]language.Language{{Code: "EN"}}, nil)
		articlesRepository.On("IncreaseView", articleUUID, increaseView).Once().Return(nil)
		articlesRepository.On("GetByCorrelationUUIDs", []string{}, "EN").Once().Return(nil, nil)
		defer articlesRepository.AssertExpectations(t)

		userRepository.On("GetOne", authorUUID).Once().Return(user.User{}, domain.ErrNotExists)
		userRepository.On("GetByUUIDs", []string{}).Once().Return([]user.User{}, nil)
		defer userRepository.AssertExpectations(t)

		mockComponent.On("Items").Once().Return([]component.Item{})
		mockComponent.On("Type").Return(component.ComponentTypeMock)
		defer mockComponent.AssertExpectations(t)

		v := []domainElement.Element{
			{Body: &mockComponent},
		}

		elementsRepository.On("GetByVenues", venues).Once().Return(v, nil)
		defer elementsRepository.AssertExpectations(t)

		elementRetriever := element.NewRetriever(&articlesRepository, &elementsRepository, &userRepository)
		response, err := NewUseCase(&articlesRepository, &userRepository, &languageResolver, elementRetriever, &requestValidator).Execute(&request)

		assert.NotNil(t, response, "unexpected response")
		assert.NoError(t, err, "unexpected error")
	})

	t.Run("error on getting elements", func(t *testing.T) {
		var (
			articlesRepository articles.MockArticlesRepository
			elementsRepository elements.MockElementsRepository
			userRepository     users.MockUsersRepository
			languageResolver   resolver.MockResolver
			requestValidator   validator.MockValidator

			correlationUUID string   = "test-correlation-uuid"
			articleUUID     string   = "test-uuid"
			authorUUID      string   = "author-uuid"
			venues          []string = []string{"articles/*", fmt.Sprintf("articles/%s", correlationUUID)}
			expectedErr              = domain.ErrNotExists

			a = article.Article{
				UUID:            articleUUID,
				AuthorUUID:      authorUUID,
				LanguageCode:    "EN",
				CorrelationUUID: correlationUUID,
			}
			au = user.User{UUID: authorUUID}

			request = Request{CorrelationUUID: correlationUUID}
		)

		requestValidator.On("Validate", &request).Once().Return(nil)
		defer requestValidator.AssertExpectations(t)

		languageResolver.On("DefaultCode").Once().Return("EN", nil)
		languageResolver.On("Resolve", "EN").Once().Return(language.Language{Code: "EN"}, nil)
		defer languageResolver.AssertExpectations(t)

		articlesRepository.On("GetOnePublished", correlationUUID, "EN").Once().Return(a, nil)
		articlesRepository.On("GetPublishedLanguages", correlationUUID).Once().Return([]language.Language{{Code: "EN"}}, nil)
		defer articlesRepository.AssertExpectations(t)

		userRepository.On("GetOne", authorUUID).Once().Return(au, nil)
		defer userRepository.AssertExpectations(t)

		elementsRepository.On("GetByVenues", venues).Once().Return(nil, expectedErr)
		defer elementsRepository.AssertExpectations(t)

		elementRetriever := element.NewRetriever(&articlesRepository, &elementsRepository, &userRepository)
		usecase := NewUseCase(&articlesRepository, &userRepository, &languageResolver, elementRetriever, &requestValidator)
		response, err := usecase.Execute(&request)

		articlesRepository.AssertNotCalled(t, "GetByCorrelationUUIDs")
		articlesRepository.AssertNotCalled(t, "IncreaseView")

		assert.Nil(t, response, "unexpected response")
		assert.ErrorIs(t, err, expectedErr)
	})

	t.Run("error on getting element articles", func(t *testing.T) {
		var (
			articlesRepository articles.MockArticlesRepository
			elementsRepository elements.MockElementsRepository
			userRepository     users.MockUsersRepository
			languageResolver   resolver.MockResolver
			requestValidator   validator.MockValidator
			mockComponent      component.MockComponent

			correlationUUID string   = "test-correlation-uuid"
			articleUUID     string   = "test-uuid"
			authorUUID      string   = "author-uuid"
			venues          []string = []string{"articles/*", fmt.Sprintf("articles/%s", correlationUUID)}
			expectedErr              = domain.ErrNotExists

			a = article.Article{
				UUID:            articleUUID,
				AuthorUUID:      authorUUID,
				LanguageCode:    "EN",
				CorrelationUUID: correlationUUID,
			}
			au = user.User{UUID: authorUUID}
			va = []article.Article{
				{UUID: "test-article-1"},
				{UUID: "test-article-2"},
			}
			i = []component.Item{
				{ContentUUID: va[0].UUID},
				{ContentUUID: va[1].UUID},
				{ContentUUID: "not-exist-article-uuid"},
			}
			u = []string{
				i[0].ContentUUID,
				i[1].ContentUUID,
				i[2].ContentUUID,
			}

			request = Request{CorrelationUUID: correlationUUID}
		)

		requestValidator.On("Validate", &request).Once().Return(nil)
		defer requestValidator.AssertExpectations(t)

		languageResolver.On("DefaultCode").Once().Return("EN", nil)
		languageResolver.On("Resolve", "EN").Once().Return(language.Language{Code: "EN"}, nil)
		defer languageResolver.AssertExpectations(t)

		articlesRepository.On("GetOnePublished", correlationUUID, "EN").Once().Return(a, nil)
		articlesRepository.On("GetPublishedLanguages", correlationUUID).Once().Return([]language.Language{{Code: "EN"}}, nil)
		articlesRepository.On("GetByCorrelationUUIDs", u, "EN").Once().Return(nil, expectedErr)
		defer articlesRepository.AssertExpectations(t)

		userRepository.On("GetOne", authorUUID).Once().Return(au, nil)
		defer userRepository.AssertExpectations(t)

		mockComponent.On("Items").Once().Return(i)
		defer mockComponent.AssertExpectations(t)

		v := []domainElement.Element{
			{Body: &mockComponent},
		}

		elementsRepository.On("GetByVenues", venues).Once().Return(v, nil)
		defer elementsRepository.AssertExpectations(t)

		elementRetriever := element.NewRetriever(&articlesRepository, &elementsRepository, &userRepository)
		response, err := NewUseCase(&articlesRepository, &userRepository, &languageResolver, elementRetriever, &requestValidator).Execute(&request)

		articlesRepository.AssertNotCalled(t, "IncreaseView")

		assert.Nil(t, response, "unexpected response")
		assert.ErrorIs(t, err, expectedErr)
	})

	t.Run("error on increasing view count is not reflected on response", func(t *testing.T) {
		var (
			articlesRepository articles.MockArticlesRepository
			elementsRepository elements.MockElementsRepository
			userRepository     users.MockUsersRepository
			languageResolver   resolver.MockResolver
			requestValidator   validator.MockValidator
			mockComponent      component.MockComponent

			correlationUUID string   = "test-correlation-uuid"
			articleUUID     string   = "test-uuid"
			authorUUID      string   = "author-uuid"
			venues          []string = []string{"articles/*", fmt.Sprintf("articles/%s", correlationUUID)}
			increaseView    uint     = 1
			expectedErr              = domain.ErrNotExists

			a = article.Article{
				UUID:            articleUUID,
				AuthorUUID:      authorUUID,
				LanguageCode:    "EN",
				CorrelationUUID: correlationUUID,
			}
			au = user.User{UUID: authorUUID}
			va = []article.Article{
				{UUID: "test-article-1"},
				{UUID: "test-article-2"},
			}
			i = []component.Item{
				{ContentUUID: va[0].UUID},
				{ContentUUID: va[1].UUID},
				{ContentUUID: "not-exist-article-uuid"},
			}
			u = []string{
				i[0].ContentUUID,
				i[1].ContentUUID,
				i[2].ContentUUID,
			}

			request = Request{CorrelationUUID: correlationUUID}
		)

		requestValidator.On("Validate", &request).Once().Return(nil)
		defer requestValidator.AssertExpectations(t)

		languageResolver.On("DefaultCode").Once().Return("EN", nil)
		languageResolver.On("Resolve", "EN").Once().Return(language.Language{Code: "EN"}, nil)
		defer languageResolver.AssertExpectations(t)

		articlesRepository.On("GetOnePublished", correlationUUID, "EN").Once().Return(a, nil)
		articlesRepository.On("GetPublishedLanguages", correlationUUID).Once().Return([]language.Language{{Code: "EN"}}, nil)
		articlesRepository.On("IncreaseView", articleUUID, increaseView).Once().Return(expectedErr)
		articlesRepository.On("GetByCorrelationUUIDs", u, "EN").Once().Return(va, nil)
		defer articlesRepository.AssertExpectations(t)

		userRepository.On("GetOne", authorUUID).Once().Return(au, nil)
		userRepository.On("GetByUUIDs", []string{"", ""}).Once().Return([]user.User{}, nil)
		defer userRepository.AssertExpectations(t)

		mockComponent.On("Items").Once().Return(i)
		mockComponent.On("Type").Return(component.ComponentTypeMock)
		defer mockComponent.AssertExpectations(t)

		v := []domainElement.Element{
			{Body: &mockComponent},
		}

		elementsRepository.On("GetByVenues", venues).Once().Return(v, nil)
		defer elementsRepository.AssertExpectations(t)

		elementRetriever := element.NewRetriever(&articlesRepository, &elementsRepository, &userRepository)
		response, err := NewUseCase(&articlesRepository, &userRepository, &languageResolver, elementRetriever, &requestValidator).Execute(&request)

		assert.NotNil(t, response, "unexpected response")
		assert.NoError(t, err, "unexpected error")
	})

	t.Run("returns the article for the requested language", func(t *testing.T) {
		var (
			articlesRepository articles.MockArticlesRepository
			elementsRepository elements.MockElementsRepository
			userRepository     users.MockUsersRepository
			languageResolver   resolver.MockResolver
			requestValidator   validator.MockValidator

			correlationUUID = "translation-uuid"

			faArticle = article.Article{
				UUID:            "fa-uuid",
				AuthorUUID:      "author-fa",
				LanguageCode:    "FA",
				CorrelationUUID: correlationUUID,
			}
			au = user.User{UUID: "author-fa", Name: "fa-author"}

			request = Request{CorrelationUUID: correlationUUID, LanguageCode: "FA"}
		)

		requestValidator.On("Validate", &request).Once().Return(nil)
		defer requestValidator.AssertExpectations(t)

		languageResolver.On("Resolve", "FA").Once().Return(language.Language{Code: "FA"}, nil)
		defer languageResolver.AssertExpectations(t)

		articlesRepository.On("GetOnePublished", correlationUUID, "FA").Once().Return(faArticle, nil)
		articlesRepository.On("GetPublishedLanguages", correlationUUID).Once().Return([]language.Language{{Code: "EN"}, {Code: "FA"}}, nil)
		articlesRepository.On("IncreaseView", "fa-uuid", uint(1)).Once().Return(nil)
		articlesRepository.On("GetByCorrelationUUIDs", []string{}, "FA").Once().Return(nil, nil)
		defer articlesRepository.AssertExpectations(t)

		userRepository.On("GetOne", "author-fa").Once().Return(au, nil)
		userRepository.On("GetByUUIDs", []string{}).Once().Return([]user.User{}, nil)
		defer userRepository.AssertExpectations(t)

		elementsRepository.On(
			"GetByVenues",
			[]string{"articles/*", "articles/translation-uuid"},
		).Once().Return([]domainElement.Element{}, nil)
		defer elementsRepository.AssertExpectations(t)

		elementRetriever := element.NewRetriever(&articlesRepository, &elementsRepository, &userRepository)
		response, err := NewUseCase(&articlesRepository, &userRepository, &languageResolver, elementRetriever, &requestValidator).Execute(&request)

		assert.NoError(t, err)
		assert.NotNil(t, response)
		assert.Equal(t, correlationUUID, response.CorrelationUUID)
		assert.Equal(t, "FA", response.LanguageCode.Code)
		assert.Len(t, response.AvailableLanguages, 2)
	})
}
