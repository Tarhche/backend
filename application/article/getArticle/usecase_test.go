package getarticle

import (
	"context"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"

	"github.com/khanzadimahdi/testproject/application/element"
	"github.com/khanzadimahdi/testproject/application/language/resolver"
	"github.com/khanzadimahdi/testproject/domain"
	"github.com/khanzadimahdi/testproject/domain/article"
	domainElement "github.com/khanzadimahdi/testproject/domain/element"
	"github.com/khanzadimahdi/testproject/domain/element/component"
	"github.com/khanzadimahdi/testproject/domain/language"
	"github.com/khanzadimahdi/testproject/domain/user"
	"github.com/khanzadimahdi/testproject/infrastructure/matcher"
	"github.com/khanzadimahdi/testproject/infrastructure/repository/mocks/articles"
	"github.com/khanzadimahdi/testproject/infrastructure/repository/mocks/elements"
	"github.com/khanzadimahdi/testproject/infrastructure/repository/mocks/languages"
	"github.com/khanzadimahdi/testproject/infrastructure/repository/mocks/users"
	"github.com/khanzadimahdi/testproject/infrastructure/validator"
)

func TestUseCase_Execute(t *testing.T) {
	t.Run("returns an article", func(t *testing.T) {
		var (
			articlesRepository  articles.MockArticlesRepository
			elementsRepository  elements.MockElementsRepository
			userRepository      users.MockUsersRepository
			languagesRepository languages.MockLanguagesRepository
			languageResolver    resolver.MockResolver
			requestValidator    validator.MockValidator
			mockComponent       component.MockComponent

			correlationUUID string   = "test-correlation-uuid"
			articleUUID     string   = "test-uuid"
			authorUUID      string   = "author-uuid"
			venues          []string = []string{fmt.Sprintf("/EN/articles/%s", correlationUUID)}
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

		languageResolver.On("DefaultCode", mock.Anything).Once().Return("EN", nil)
		languageResolver.On("Resolve", mock.Anything, "EN").Once().Return(language.Language{Code: "EN"}, nil)
		defer languageResolver.AssertExpectations(t)

		articlesRepository.On("GetOnePublished", mock.Anything, correlationUUID, "EN").Once().Return(a, nil)
		articlesRepository.On("GetPublishedLanguageCodes", mock.Anything, correlationUUID).Once().Return([]string{"EN"}, nil)
		languagesRepository.On("GetByCodes", mock.Anything, []string{"EN"}).Once().Return([]language.Language{{Code: "EN"}}, nil)
		articlesRepository.On("IncreaseView", mock.Anything, articleUUID, increaseView).Once().Return(nil)
		articlesRepository.On("GetByCorrelationUUIDs", mock.Anything, u, "EN").Once().Return(va, nil)
		defer articlesRepository.AssertExpectations(t)

		userRepository.On("GetOne", mock.Anything, authorUUID).Once().Return(au, nil)
		userRepository.On("GetByUUIDs", mock.Anything, []string{"author-uuid-1", "author-uuid-2"}).Once().Return(elementUsers, nil)
		defer userRepository.AssertExpectations(t)

		mockComponent.On("Items").Once().Return(i)
		mockComponent.On("Type").Return(component.ComponentTypeMock)
		defer mockComponent.AssertExpectations(t)

		v := []domainElement.Element{
			{Body: &mockComponent, Venues: venues},
		}

		elementsRepository.On("Count", mock.Anything).Once().Return(uint(len(v)), nil)
		elementsRepository.On("GetAll", mock.Anything, uint(0), uint(len(v))).Once().Return(v, nil)
		defer elementsRepository.AssertExpectations(t)

		elementRetriever := element.NewRetriever(&articlesRepository, &elementsRepository, &userRepository, matcher.New())
		response, err := NewUseCase(&articlesRepository, &userRepository, &languagesRepository, &languageResolver, elementRetriever, &requestValidator).Execute(context.Background(), &request)

		assert.NotNil(t, response, "unexpected response")
		assert.NoError(t, err, "unexpected error")
	})

	t.Run("error on getting article", func(t *testing.T) {
		var (
			articlesRepository  articles.MockArticlesRepository
			elementsRepository  elements.MockElementsRepository
			userRepository      users.MockUsersRepository
			languagesRepository languages.MockLanguagesRepository
			languageResolver    resolver.MockResolver
			requestValidator    validator.MockValidator

			correlationUUID string = "test-correlation-uuid"
			expectedErr            = domain.ErrNotExists

			request = Request{CorrelationUUID: correlationUUID}
		)

		requestValidator.On("Validate", &request).Once().Return(nil)
		defer requestValidator.AssertExpectations(t)

		languageResolver.On("DefaultCode", mock.Anything).Once().Return("EN", nil)
		languageResolver.On("Resolve", mock.Anything, "EN").Once().Return(language.Language{Code: "EN"}, nil)
		defer languageResolver.AssertExpectations(t)

		articlesRepository.On("GetOnePublished", mock.Anything, correlationUUID, "EN").Once().Return(article.Article{}, expectedErr)
		defer articlesRepository.AssertExpectations(t)

		elementRetriever := element.NewRetriever(&articlesRepository, &elementsRepository, &userRepository, matcher.New())
		usecase := NewUseCase(&articlesRepository, &userRepository, &languagesRepository, &languageResolver, elementRetriever, &requestValidator)
		response, err := usecase.Execute(context.Background(), &request)

		userRepository.AssertNotCalled(t, "GetOne")
		articlesRepository.AssertNotCalled(t, "GetPublishedLanguageCodes")
		elementsRepository.AssertNotCalled(t, "GetAll")
		articlesRepository.AssertNotCalled(t, "GetByCorrelationUUIDs")
		articlesRepository.AssertNotCalled(t, "IncreaseView")

		assert.Nil(t, response, "unexpected response")
		assert.ErrorIs(t, err, expectedErr)
	})

	t.Run("missing author is handled gracefully", func(t *testing.T) {
		var (
			articlesRepository  articles.MockArticlesRepository
			elementsRepository  elements.MockElementsRepository
			userRepository      users.MockUsersRepository
			languagesRepository languages.MockLanguagesRepository
			languageResolver    resolver.MockResolver
			requestValidator    validator.MockValidator
			mockComponent       component.MockComponent

			correlationUUID string   = "test-correlation-uuid"
			articleUUID     string   = "test-uuid"
			authorUUID      string   = "missing-author-uuid"
			venues          []string = []string{fmt.Sprintf("/EN/articles/%s", correlationUUID)}
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

		languageResolver.On("DefaultCode", mock.Anything).Once().Return("EN", nil)
		languageResolver.On("Resolve", mock.Anything, "EN").Once().Return(language.Language{Code: "EN"}, nil)
		defer languageResolver.AssertExpectations(t)

		articlesRepository.On("GetOnePublished", mock.Anything, correlationUUID, "EN").Once().Return(a, nil)
		articlesRepository.On("GetPublishedLanguageCodes", mock.Anything, correlationUUID).Once().Return([]string{"EN"}, nil)
		languagesRepository.On("GetByCodes", mock.Anything, []string{"EN"}).Once().Return([]language.Language{{Code: "EN"}}, nil)
		articlesRepository.On("IncreaseView", mock.Anything, articleUUID, increaseView).Once().Return(nil)
		articlesRepository.On("GetByCorrelationUUIDs", mock.Anything, []string{}, "EN").Once().Return(nil, nil)
		defer articlesRepository.AssertExpectations(t)

		userRepository.On("GetOne", mock.Anything, authorUUID).Once().Return(user.User{}, domain.ErrNotExists)
		userRepository.On("GetByUUIDs", mock.Anything, []string{}).Once().Return([]user.User{}, nil)
		defer userRepository.AssertExpectations(t)

		mockComponent.On("Items").Once().Return([]component.Item{})
		mockComponent.On("Type").Return(component.ComponentTypeMock)
		defer mockComponent.AssertExpectations(t)

		v := []domainElement.Element{
			{Body: &mockComponent, Venues: venues},
		}

		elementsRepository.On("Count", mock.Anything).Once().Return(uint(len(v)), nil)
		elementsRepository.On("GetAll", mock.Anything, uint(0), uint(len(v))).Once().Return(v, nil)
		defer elementsRepository.AssertExpectations(t)

		elementRetriever := element.NewRetriever(&articlesRepository, &elementsRepository, &userRepository, matcher.New())
		response, err := NewUseCase(&articlesRepository, &userRepository, &languagesRepository, &languageResolver, elementRetriever, &requestValidator).Execute(context.Background(), &request)

		assert.NotNil(t, response, "unexpected response")
		assert.NoError(t, err, "unexpected error")
	})

	t.Run("error on getting elements", func(t *testing.T) {
		var (
			articlesRepository  articles.MockArticlesRepository
			elementsRepository  elements.MockElementsRepository
			userRepository      users.MockUsersRepository
			languagesRepository languages.MockLanguagesRepository
			languageResolver    resolver.MockResolver
			requestValidator    validator.MockValidator

			correlationUUID string = "test-correlation-uuid"
			articleUUID     string = "test-uuid"
			authorUUID      string = "author-uuid"
			expectedErr            = domain.ErrNotExists

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

		languageResolver.On("DefaultCode", mock.Anything).Once().Return("EN", nil)
		languageResolver.On("Resolve", mock.Anything, "EN").Once().Return(language.Language{Code: "EN"}, nil)
		defer languageResolver.AssertExpectations(t)

		articlesRepository.On("GetOnePublished", mock.Anything, correlationUUID, "EN").Once().Return(a, nil)
		articlesRepository.On("GetPublishedLanguageCodes", mock.Anything, correlationUUID).Once().Return([]string{"EN"}, nil)
		languagesRepository.On("GetByCodes", mock.Anything, []string{"EN"}).Once().Return([]language.Language{{Code: "EN"}}, nil)
		defer articlesRepository.AssertExpectations(t)

		userRepository.On("GetOne", mock.Anything, authorUUID).Once().Return(au, nil)
		defer userRepository.AssertExpectations(t)

		elementsRepository.On("Count", mock.Anything).Once().Return(uint(1), nil)
		elementsRepository.On("GetAll", mock.Anything, uint(0), uint(1)).Once().Return(nil, expectedErr)
		defer elementsRepository.AssertExpectations(t)

		elementRetriever := element.NewRetriever(&articlesRepository, &elementsRepository, &userRepository, matcher.New())
		usecase := NewUseCase(&articlesRepository, &userRepository, &languagesRepository, &languageResolver, elementRetriever, &requestValidator)
		response, err := usecase.Execute(context.Background(), &request)

		articlesRepository.AssertNotCalled(t, "GetByCorrelationUUIDs")
		articlesRepository.AssertNotCalled(t, "IncreaseView")

		assert.Nil(t, response, "unexpected response")
		assert.ErrorIs(t, err, expectedErr)
	})

	t.Run("error on getting element articles", func(t *testing.T) {
		var (
			articlesRepository  articles.MockArticlesRepository
			elementsRepository  elements.MockElementsRepository
			userRepository      users.MockUsersRepository
			languagesRepository languages.MockLanguagesRepository
			languageResolver    resolver.MockResolver
			requestValidator    validator.MockValidator
			mockComponent       component.MockComponent

			correlationUUID string   = "test-correlation-uuid"
			articleUUID     string   = "test-uuid"
			authorUUID      string   = "author-uuid"
			venues          []string = []string{fmt.Sprintf("/EN/articles/%s", correlationUUID)}
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

		languageResolver.On("DefaultCode", mock.Anything).Once().Return("EN", nil)
		languageResolver.On("Resolve", mock.Anything, "EN").Once().Return(language.Language{Code: "EN"}, nil)
		defer languageResolver.AssertExpectations(t)

		articlesRepository.On("GetOnePublished", mock.Anything, correlationUUID, "EN").Once().Return(a, nil)
		articlesRepository.On("GetPublishedLanguageCodes", mock.Anything, correlationUUID).Once().Return([]string{"EN"}, nil)
		languagesRepository.On("GetByCodes", mock.Anything, []string{"EN"}).Once().Return([]language.Language{{Code: "EN"}}, nil)
		articlesRepository.On("GetByCorrelationUUIDs", mock.Anything, u, "EN").Once().Return(nil, expectedErr)
		defer articlesRepository.AssertExpectations(t)

		userRepository.On("GetOne", mock.Anything, authorUUID).Once().Return(au, nil)
		defer userRepository.AssertExpectations(t)

		mockComponent.On("Items").Once().Return(i)
		defer mockComponent.AssertExpectations(t)

		v := []domainElement.Element{
			{Body: &mockComponent, Venues: venues},
		}

		elementsRepository.On("Count", mock.Anything).Once().Return(uint(len(v)), nil)
		elementsRepository.On("GetAll", mock.Anything, uint(0), uint(len(v))).Once().Return(v, nil)
		defer elementsRepository.AssertExpectations(t)

		elementRetriever := element.NewRetriever(&articlesRepository, &elementsRepository, &userRepository, matcher.New())
		response, err := NewUseCase(&articlesRepository, &userRepository, &languagesRepository, &languageResolver, elementRetriever, &requestValidator).Execute(context.Background(), &request)

		articlesRepository.AssertNotCalled(t, "IncreaseView")

		assert.Nil(t, response, "unexpected response")
		assert.ErrorIs(t, err, expectedErr)
	})

	t.Run("error on increasing view count is not reflected on response", func(t *testing.T) {
		var (
			articlesRepository  articles.MockArticlesRepository
			elementsRepository  elements.MockElementsRepository
			userRepository      users.MockUsersRepository
			languagesRepository languages.MockLanguagesRepository
			languageResolver    resolver.MockResolver
			requestValidator    validator.MockValidator
			mockComponent       component.MockComponent

			correlationUUID string   = "test-correlation-uuid"
			articleUUID     string   = "test-uuid"
			authorUUID      string   = "author-uuid"
			venues          []string = []string{fmt.Sprintf("/EN/articles/%s", correlationUUID)}
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

		languageResolver.On("DefaultCode", mock.Anything).Once().Return("EN", nil)
		languageResolver.On("Resolve", mock.Anything, "EN").Once().Return(language.Language{Code: "EN"}, nil)
		defer languageResolver.AssertExpectations(t)

		articlesRepository.On("GetOnePublished", mock.Anything, correlationUUID, "EN").Once().Return(a, nil)
		articlesRepository.On("GetPublishedLanguageCodes", mock.Anything, correlationUUID).Once().Return([]string{"EN"}, nil)
		languagesRepository.On("GetByCodes", mock.Anything, []string{"EN"}).Once().Return([]language.Language{{Code: "EN"}}, nil)
		articlesRepository.On("IncreaseView", mock.Anything, articleUUID, increaseView).Once().Return(expectedErr)
		articlesRepository.On("GetByCorrelationUUIDs", mock.Anything, u, "EN").Once().Return(va, nil)
		defer articlesRepository.AssertExpectations(t)

		userRepository.On("GetOne", mock.Anything, authorUUID).Once().Return(au, nil)
		userRepository.On("GetByUUIDs", mock.Anything, []string{"", ""}).Once().Return([]user.User{}, nil)
		defer userRepository.AssertExpectations(t)

		mockComponent.On("Items").Once().Return(i)
		mockComponent.On("Type").Return(component.ComponentTypeMock)
		defer mockComponent.AssertExpectations(t)

		v := []domainElement.Element{
			{Body: &mockComponent, Venues: venues},
		}

		elementsRepository.On("Count", mock.Anything).Once().Return(uint(len(v)), nil)
		elementsRepository.On("GetAll", mock.Anything, uint(0), uint(len(v))).Once().Return(v, nil)
		defer elementsRepository.AssertExpectations(t)

		elementRetriever := element.NewRetriever(&articlesRepository, &elementsRepository, &userRepository, matcher.New())
		response, err := NewUseCase(&articlesRepository, &userRepository, &languagesRepository, &languageResolver, elementRetriever, &requestValidator).Execute(context.Background(), &request)

		assert.NotNil(t, response, "unexpected response")
		assert.NoError(t, err, "unexpected error")
	})

	t.Run("returns the article for the requested language", func(t *testing.T) {
		var (
			articlesRepository  articles.MockArticlesRepository
			elementsRepository  elements.MockElementsRepository
			userRepository      users.MockUsersRepository
			languagesRepository languages.MockLanguagesRepository
			languageResolver    resolver.MockResolver
			requestValidator    validator.MockValidator

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

		languageResolver.On("Resolve", mock.Anything, "FA").Once().Return(language.Language{Code: "FA"}, nil)
		defer languageResolver.AssertExpectations(t)

		articlesRepository.On("GetOnePublished", mock.Anything, correlationUUID, "FA").Once().Return(faArticle, nil)
		articlesRepository.On("GetPublishedLanguageCodes", mock.Anything, correlationUUID).Once().Return([]string{"EN", "FA"}, nil)
		languagesRepository.On("GetByCodes", mock.Anything, []string{"EN", "FA"}).Once().Return([]language.Language{{Code: "EN"}, {Code: "FA"}}, nil)
		articlesRepository.On("IncreaseView", mock.Anything, "fa-uuid", uint(1)).Once().Return(nil)
		defer articlesRepository.AssertExpectations(t)

		userRepository.On("GetOne", mock.Anything, "author-fa").Once().Return(au, nil)
		defer userRepository.AssertExpectations(t)

		elementsRepository.On("Count", mock.Anything).Once().Return(uint(0), nil)
		defer elementsRepository.AssertExpectations(t)

		elementRetriever := element.NewRetriever(&articlesRepository, &elementsRepository, &userRepository, matcher.New())
		response, err := NewUseCase(&articlesRepository, &userRepository, &languagesRepository, &languageResolver, elementRetriever, &requestValidator).Execute(context.Background(), &request)

		assert.NoError(t, err)
		assert.NotNil(t, response)
		assert.Equal(t, correlationUUID, response.CorrelationUUID)
		assert.Equal(t, "FA", response.LanguageCode.Code)
		assert.Len(t, response.AvailableLanguages, 2)
	})
}
