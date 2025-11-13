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
	"github.com/khanzadimahdi/testproject/domain"
	"github.com/khanzadimahdi/testproject/domain/article"
	"github.com/khanzadimahdi/testproject/domain/author"
	"github.com/khanzadimahdi/testproject/infrastructure/repository/mocks/articles"
	"github.com/khanzadimahdi/testproject/infrastructure/repository/mocks/elements"
)

func TestShowHandler(t *testing.T) {
	t.Parallel()

	t.Run("show", func(t *testing.T) {
		t.Parallel()

		var (
			articlesRepository articles.MockArticlesRepository
			elementsRepository elements.MockElementsRepository
		)

		publishedAt, err := time.Parse(time.RFC3339, "2024-09-29T15:56:25Z")
		assert.NoError(t, err)

		a := article.Article{
			UUID:        "test-uuid-1",
			Title:       "test title",
			Body:        "test body",
			PublishedAt: publishedAt,
			Author: author.Author{
				UUID:   "author-uuid",
				Name:   "test name",
				Avatar: "test avatar",
			},
			ViewCount: 11,
		}

		articlesRepository.On("GetOnePublished", a.UUID).Once().Return(a, nil)
		articlesRepository.On("IncreaseView", a.UUID, uint(1)).Once().Return(nil)
		articlesRepository.On("GetByUUIDs", []string{}).Once().Return(nil, nil)
		defer articlesRepository.AssertExpectations(t)

		elementsRepository.On("GetByVenues", []string{fmt.Sprintf("articles/%s", a.UUID)}).Once().Return(nil, nil)
		defer elementsRepository.AssertExpectations(t)

		elementRetriever := element.NewRetriever(&articlesRepository, &elementsRepository)
		handler := NewShowHandler(getarticle.NewUseCase(&articlesRepository, elementRetriever))

		request := httptest.NewRequest(http.MethodGet, "/", nil)
		request.SetPathValue("uuid", a.UUID)

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
		)

		a := article.Article{
			UUID: "test-uuid-1",
		}

		articlesRepository.On("GetOnePublished", a.UUID).Once().Return(article.Article{}, domain.ErrNotExists)
		defer articlesRepository.AssertExpectations(t)

		elementRetriever := element.NewRetriever(&articlesRepository, &elementsRepository)
		handler := NewShowHandler(getarticle.NewUseCase(&articlesRepository, elementRetriever))

		request := httptest.NewRequest(http.MethodGet, "/", nil)
		request.SetPathValue("uuid", a.UUID)

		response := httptest.NewRecorder()

		handler.ServeHTTP(response, request)

		articlesRepository.AssertNotCalled(t, "IncreaseView")
		articlesRepository.AssertNotCalled(t, "GetByUUIDs")
		elementsRepository.AssertNotCalled(t, "GetByVenues")

		assert.Len(t, response.Body.Bytes(), 0)
		assert.Equal(t, http.StatusNotFound, response.Code)
	})

	t.Run("error", func(t *testing.T) {
		t.Parallel()

		var (
			articlesRepository articles.MockArticlesRepository
			elementsRepository elements.MockElementsRepository
		)

		a := article.Article{
			UUID: "test-uuid-1",
		}

		articlesRepository.On("GetOnePublished", a.UUID).Once().Return(article.Article{}, errors.New("an error has happened"))
		defer articlesRepository.AssertExpectations(t)

		elementRetriever := element.NewRetriever(&articlesRepository, &elementsRepository)
		handler := NewShowHandler(getarticle.NewUseCase(&articlesRepository, elementRetriever))

		request := httptest.NewRequest(http.MethodGet, "/", nil)
		request.SetPathValue("uuid", a.UUID)

		response := httptest.NewRecorder()

		handler.ServeHTTP(response, request)

		articlesRepository.AssertNotCalled(t, "IncreaseView")
		articlesRepository.AssertNotCalled(t, "GetByUUIDs")
		elementsRepository.AssertNotCalled(t, "GetByVenues")

		assert.Len(t, response.Body.Bytes(), 0)
		assert.Equal(t, http.StatusInternalServerError, response.Code)
	})
}
