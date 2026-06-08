package article

import (
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	getarticle "github.com/khanzadimahdi/testproject/application/article/getArticle"
	"github.com/khanzadimahdi/testproject/application/element"
	"github.com/khanzadimahdi/testproject/application/language/resolver"
	"github.com/khanzadimahdi/testproject/domain"
	"github.com/khanzadimahdi/testproject/domain/article"
	"github.com/khanzadimahdi/testproject/domain/language"
	"github.com/khanzadimahdi/testproject/domain/user"
	"github.com/khanzadimahdi/testproject/infrastructure/repository/mocks/articles"
	"github.com/khanzadimahdi/testproject/infrastructure/repository/mocks/elements"
	"github.com/khanzadimahdi/testproject/infrastructure/repository/mocks/users"
	"github.com/khanzadimahdi/testproject/infrastructure/validator"
)

func TestShowHandler(t *testing.T) {
	t.Parallel()

	t.Run("show", func(t *testing.T) {
		t.Parallel()

		var (
			articlesRepository articles.MockArticlesRepository
			elementsRepository elements.MockElementsRepository
			userRepository     users.MockUsersRepository
			languageResolver   resolver.MockResolver
			requestValidator   validator.MockValidator
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

		languageResolver.On("DefaultCode").Once().Return("EN", nil)
		languageResolver.On("Resolve", "EN").Once().Return(language.Language{Code: "EN", Name: "English"}, nil)
		defer languageResolver.AssertExpectations(t)

		articlesRepository.On("GetOnePublished", a.CorrelationUUID, "EN").Once().Return(a, nil)
		articlesRepository.On("GetPublishedLanguages", a.CorrelationUUID).Once().Return([]language.Language{{Code: "EN", Name: "English"}}, nil)
		articlesRepository.On("IncreaseView", a.UUID, uint(1)).Once().Return(nil)
		articlesRepository.On("GetByCorrelationUUIDs", []string{}, "EN").Once().Return(nil, nil)
		defer articlesRepository.AssertExpectations(t)

		userRepository.On("GetOne", "author-uuid").Once().Return(u, nil)
		userRepository.On("GetByUUIDs", []string{}).Once().Return([]user.User{}, nil)
		defer userRepository.AssertExpectations(t)

		elementsRepository.On("GetByVenues", []string{"articles/*", fmt.Sprintf("articles/%s", a.UUID)}).Once().Return(nil, nil)
		defer elementsRepository.AssertExpectations(t)

		elementRetriever := element.NewRetriever(&articlesRepository, &elementsRepository, &userRepository)
		handler := NewShowHandler(getarticle.NewUseCase(&articlesRepository, &userRepository, &languageResolver, elementRetriever, &requestValidator))

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
			articlesRepository articles.MockArticlesRepository
			elementsRepository elements.MockElementsRepository
			userRepository     users.MockUsersRepository
			languageResolver   resolver.MockResolver
			requestValidator   validator.MockValidator

			correlationUUID = "test-uuid-1"
		)

		requestValidator.On("Validate", &getarticle.Request{CorrelationUUID: correlationUUID}).Once().Return(nil)
		defer requestValidator.AssertExpectations(t)

		languageResolver.On("DefaultCode").Once().Return("EN", nil)
		languageResolver.On("Resolve", "EN").Once().Return(language.Language{Code: "EN"}, nil)
		defer languageResolver.AssertExpectations(t)

		articlesRepository.On("GetOnePublished", correlationUUID, "EN").Once().Return(article.Article{}, domain.ErrNotExists)
		defer articlesRepository.AssertExpectations(t)

		elementRetriever := element.NewRetriever(&articlesRepository, &elementsRepository, &userRepository)
		handler := NewShowHandler(getarticle.NewUseCase(&articlesRepository, &userRepository, &languageResolver, elementRetriever, &requestValidator))

		request := httptest.NewRequest(http.MethodGet, "/", nil)
		request.SetPathValue("uuid", correlationUUID)

		response := httptest.NewRecorder()

		handler.ServeHTTP(response, request)

		articlesRepository.AssertNotCalled(t, "IncreaseView")
		articlesRepository.AssertNotCalled(t, "GetByCorrelationUUIDs")
		userRepository.AssertNotCalled(t, "GetOne")
		elementsRepository.AssertNotCalled(t, "GetByVenues")

		assert.Len(t, response.Body.Bytes(), 0)
		assert.Equal(t, http.StatusNotFound, response.Code)
	})

	t.Run("error", func(t *testing.T) {
		t.Parallel()

		var (
			articlesRepository articles.MockArticlesRepository
			elementsRepository elements.MockElementsRepository
			userRepository     users.MockUsersRepository
			languageResolver   resolver.MockResolver
			requestValidator   validator.MockValidator

			correlationUUID = "test-uuid-1"
		)

		requestValidator.On("Validate", &getarticle.Request{CorrelationUUID: correlationUUID}).Once().Return(nil)
		defer requestValidator.AssertExpectations(t)

		languageResolver.On("DefaultCode").Once().Return("EN", nil)
		languageResolver.On("Resolve", "EN").Once().Return(language.Language{Code: "EN"}, nil)
		defer languageResolver.AssertExpectations(t)

		articlesRepository.On("GetOnePublished", correlationUUID, "EN").Once().Return(article.Article{}, errors.New("an error has happened"))
		defer articlesRepository.AssertExpectations(t)

		elementRetriever := element.NewRetriever(&articlesRepository, &elementsRepository, &userRepository)
		handler := NewShowHandler(getarticle.NewUseCase(&articlesRepository, &userRepository, &languageResolver, elementRetriever, &requestValidator))

		request := httptest.NewRequest(http.MethodGet, "/", nil)
		request.SetPathValue("uuid", correlationUUID)

		response := httptest.NewRecorder()

		handler.ServeHTTP(response, request)

		articlesRepository.AssertNotCalled(t, "IncreaseView")
		articlesRepository.AssertNotCalled(t, "GetByCorrelationUUIDs")
		userRepository.AssertNotCalled(t, "GetOne")
		elementsRepository.AssertNotCalled(t, "GetByVenues")

		assert.Len(t, response.Body.Bytes(), 0)
		assert.Equal(t, http.StatusInternalServerError, response.Code)
	})
}
