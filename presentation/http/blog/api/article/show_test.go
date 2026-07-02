package article

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"

	getarticle "github.com/khanzadimahdi/testproject/application/article/getArticle"
	"github.com/khanzadimahdi/testproject/application/element"
	"github.com/khanzadimahdi/testproject/application/language/resolver"
	"github.com/khanzadimahdi/testproject/domain"
	"github.com/khanzadimahdi/testproject/domain/article"
	"github.com/khanzadimahdi/testproject/domain/language"
	"github.com/khanzadimahdi/testproject/domain/user"
	"github.com/khanzadimahdi/testproject/infrastructure/matcher"
	"github.com/khanzadimahdi/testproject/infrastructure/repository/mocks/articles"
	"github.com/khanzadimahdi/testproject/infrastructure/repository/mocks/elements"
	"github.com/khanzadimahdi/testproject/infrastructure/repository/mocks/languages"
	"github.com/khanzadimahdi/testproject/infrastructure/repository/mocks/users"
	"github.com/khanzadimahdi/testproject/infrastructure/validator"
)

func TestShowHandler(t *testing.T) {
	t.Parallel()

	t.Run("show", func(t *testing.T) {
		t.Parallel()

		var (
			articlesRepository  articles.MockArticlesRepository
			elementsRepository  elements.MockElementsRepository
			userRepository      users.MockUsersRepository
			languagesRepository languages.MockLanguagesRepository
			languageResolver    resolver.MockResolver
			requestValidator    validator.MockValidator
		)

		publishedAt, err := time.Parse(time.RFC3339, "2024-09-29T15:56:25Z")
		assert.NoError(t, err)

		a := article.Article{
			UUID:            "test-uuid-1",
			Title:           "test title",
			Body:            "test body",
			PublishedAt:     publishedAt,
			AuthorUUID:      "author-uuid",
			ViewCount:       11,
			LanguageCode:    "EN",
			CorrelationUUID: "test-uuid-1",
		}
		u := user.User{
			UUID:     "author-uuid",
			Name:     "test name",
			Avatar:   "test avatar",
			Username: "author-username",
		}

		requestValidator.On("Validate", &getarticle.Request{CorrelationUUID: a.CorrelationUUID}).Once().Return(nil)
		defer requestValidator.AssertExpectations(t)

		languageResolver.On("DefaultCode", mock.Anything).Once().Return("EN", nil)
		languageResolver.On("Resolve", mock.Anything, "EN").Once().Return(language.Language{Code: "EN", Name: "English"}, nil)
		defer languageResolver.AssertExpectations(t)

		articlesRepository.On("GetOnePublished", mock.Anything, a.CorrelationUUID, "EN").Once().Return(a, nil)
		articlesRepository.On("GetPublishedLanguageCodes", mock.Anything, a.CorrelationUUID).Once().Return([]string{"EN"}, nil)
		languagesRepository.On("GetByCodes", mock.Anything, []string{"EN"}).Once().Return([]language.Language{{Code: "EN", Name: "English"}}, nil)
		articlesRepository.On("IncreaseView", mock.Anything, a.UUID, uint(1)).Once().Return(nil)
		defer articlesRepository.AssertExpectations(t)

		userRepository.On("GetOne", mock.Anything, "author-uuid").Once().Return(u, nil)
		defer userRepository.AssertExpectations(t)

		elementsRepository.On("Count", mock.Anything).Once().Return(uint(0), nil)
		defer elementsRepository.AssertExpectations(t)

		elementRetriever := element.NewRetriever(&articlesRepository, &elementsRepository, &userRepository, matcher.New())
		handler := NewShowHandler(getarticle.NewUseCase(&articlesRepository, &userRepository, &languagesRepository, &languageResolver, elementRetriever, &requestValidator))

		request := httptest.NewRequest(http.MethodGet, "/", nil)
		request.SetPathValue("uuid", a.CorrelationUUID)

		response := httptest.NewRecorder()

		handler.ServeHTTP(response, request)

		expected, err := os.ReadFile("testdata/response-show.json")
		assert.NoError(t, err)

		assert.Equal(t, "application/json", response.Header().Get("content-type"))
		assert.JSONEq(t, string(expected), response.Body.String())
		assert.Equal(t, http.StatusOK, response.Code)
	})

	t.Run("not found", func(t *testing.T) {
		t.Parallel()

		var (
			articlesRepository  articles.MockArticlesRepository
			elementsRepository  elements.MockElementsRepository
			userRepository      users.MockUsersRepository
			languagesRepository languages.MockLanguagesRepository
			languageResolver    resolver.MockResolver
			requestValidator    validator.MockValidator

			correlationUUID = "test-uuid-1"
		)

		requestValidator.On("Validate", &getarticle.Request{CorrelationUUID: correlationUUID}).Once().Return(nil)
		defer requestValidator.AssertExpectations(t)

		languageResolver.On("DefaultCode", mock.Anything).Once().Return("EN", nil)
		languageResolver.On("Resolve", mock.Anything, "EN").Once().Return(language.Language{Code: "EN"}, nil)
		defer languageResolver.AssertExpectations(t)

		articlesRepository.On("GetOnePublished", mock.Anything, correlationUUID, "EN").Once().Return(article.Article{}, domain.ErrNotExists)
		defer articlesRepository.AssertExpectations(t)

		elementRetriever := element.NewRetriever(&articlesRepository, &elementsRepository, &userRepository, matcher.New())
		handler := NewShowHandler(getarticle.NewUseCase(&articlesRepository, &userRepository, &languagesRepository, &languageResolver, elementRetriever, &requestValidator))

		request := httptest.NewRequest(http.MethodGet, "/", nil)
		request.SetPathValue("uuid", correlationUUID)

		response := httptest.NewRecorder()

		handler.ServeHTTP(response, request)

		articlesRepository.AssertNotCalled(t, "IncreaseView")
		articlesRepository.AssertNotCalled(t, "GetByCorrelationUUIDs")
		userRepository.AssertNotCalled(t, "GetOne")
		elementsRepository.AssertNotCalled(t, "Count")

		assert.Len(t, response.Body.Bytes(), 0)
		assert.Equal(t, http.StatusNotFound, response.Code)
	})

	t.Run("error", func(t *testing.T) {
		t.Parallel()

		var (
			articlesRepository  articles.MockArticlesRepository
			elementsRepository  elements.MockElementsRepository
			userRepository      users.MockUsersRepository
			languagesRepository languages.MockLanguagesRepository
			languageResolver    resolver.MockResolver
			requestValidator    validator.MockValidator

			correlationUUID = "test-uuid-1"
		)

		requestValidator.On("Validate", &getarticle.Request{CorrelationUUID: correlationUUID}).Once().Return(nil)
		defer requestValidator.AssertExpectations(t)

		languageResolver.On("DefaultCode", mock.Anything).Once().Return("EN", nil)
		languageResolver.On("Resolve", mock.Anything, "EN").Once().Return(language.Language{Code: "EN"}, nil)
		defer languageResolver.AssertExpectations(t)

		articlesRepository.On("GetOnePublished", mock.Anything, correlationUUID, "EN").Once().Return(article.Article{}, errors.New("an error has happened"))
		defer articlesRepository.AssertExpectations(t)

		elementRetriever := element.NewRetriever(&articlesRepository, &elementsRepository, &userRepository, matcher.New())
		handler := NewShowHandler(getarticle.NewUseCase(&articlesRepository, &userRepository, &languagesRepository, &languageResolver, elementRetriever, &requestValidator))

		request := httptest.NewRequest(http.MethodGet, "/", nil)
		request.SetPathValue("uuid", correlationUUID)

		response := httptest.NewRecorder()

		handler.ServeHTTP(response, request)

		articlesRepository.AssertNotCalled(t, "IncreaseView")
		articlesRepository.AssertNotCalled(t, "GetByCorrelationUUIDs")
		userRepository.AssertNotCalled(t, "GetOne")
		elementsRepository.AssertNotCalled(t, "Count")

		assert.Len(t, response.Body.Bytes(), 0)
		assert.Equal(t, http.StatusInternalServerError, response.Code)
	})
}
