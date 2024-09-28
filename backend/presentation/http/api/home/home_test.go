package home

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/khanzadimahdi/testproject/application/home"
	"github.com/khanzadimahdi/testproject/domain/article"
	"github.com/khanzadimahdi/testproject/domain/author"
	"github.com/khanzadimahdi/testproject/infrastructure/repository/mocks/articles"
	"github.com/khanzadimahdi/testproject/infrastructure/repository/mocks/elements"
)

func TestHomeHandler(t *testing.T) {
	t.Run("show home data", func(t *testing.T) {
		var (
			articlesRepository articles.MockArticlesRepository
			elementsRepository elements.MockElementsRepository
		)

		publishedAt, err := time.Parse(time.RFC3339, "2024-09-29T15:56:25Z")
		assert.NoError(t, err)

		articles := []article.Article{
			{
				UUID:        "article-uuid-1",
				Cover:       "article-cover-1",
				Video:       "article-video-1",
				Title:       "article-title-1",
				Excerpt:     "article-excerpt-1",
				Body:        "article-body-1",
				PublishedAt: publishedAt,
				Author: author.Author{
					UUID:   "author-uuid-1",
					Name:   "author-name",
					Avatar: "author-avatar",
				},
				Tags:      []string{"tag-1", "tag-2", "tag-3"},
				ViewCount: 123,
			},
			{
				UUID:    "article-uuid-2",
				Cover:   "article-cover-2",
				Title:   "article-title-2",
				Excerpt: "article-excerpt-2",
				Body:    "article-body-2",
				Author: author.Author{
					UUID:   "author-uuid-1",
					Name:   "author-name",
					Avatar: "author-avatar",
				},
			},
			{
				UUID:    "article-uuid-3",
				Cover:   "article-cover-3",
				Title:   "article-title-3",
				Excerpt: "article-excerpt-3",
				Body:    "article-body-3",
				Author: author.Author{
					UUID:   "author-uuid-2",
					Name:   "author-name",
					Avatar: "author-avatar",
				},
			},
		}

		articlesRepository.On("GetMostViewed", uint(4)).Once().Return(articles, nil)
		articlesRepository.On("GetAllPublished", uint(0), uint(3)).Once().Return(articles, nil)
		articlesRepository.On("GetByUUIDs", []string{}).Once().Return(nil, nil)
		defer articlesRepository.AssertExpectations(t)

		elementsRepository.On("GetByVenues", []string{"home"}).Once().Return(nil, nil)
		defer elementsRepository.AssertExpectations(t)

		useCase := home.NewUseCase(&articlesRepository, &elementsRepository)
		handler := NewHomeHandler(useCase)

		request := httptest.NewRequest(http.MethodGet, "/", nil)
		response := httptest.NewRecorder()

		handler.ServeHTTP(response, request)

		expected, err := os.ReadFile("testdata/response-01.txt")
		assert.NoError(t, err)

		assert.Equal(t, "application/json", response.Header().Get("content-type"))
		assert.JSONEq(t, string(expected), response.Body.String())
		assert.Equal(t, http.StatusOK, response.Code)
	})

	t.Run("no data", func(t *testing.T) {
		var (
			articlesRepository articles.MockArticlesRepository
			elementsRepository elements.MockElementsRepository
		)

		articlesRepository.On("GetMostViewed", uint(4)).Once().Return([]article.Article{}, nil)
		articlesRepository.On("GetAllPublished", uint(0), uint(3)).Once().Return([]article.Article{}, nil)
		articlesRepository.On("GetByUUIDs", []string{}).Once().Return([]article.Article{}, nil)
		defer articlesRepository.AssertExpectations(t)

		elementsRepository.On("GetByVenues", []string{"home"}).Once().Return(nil, nil)
		defer elementsRepository.AssertExpectations(t)

		useCase := home.NewUseCase(&articlesRepository, &elementsRepository)
		handler := NewHomeHandler(useCase)

		request := httptest.NewRequest(http.MethodGet, "/", nil)
		response := httptest.NewRecorder()

		handler.ServeHTTP(response, request)

		expected, err := os.ReadFile("testdata/response-02.txt")
		assert.NoError(t, err)

		assert.Equal(t, "application/json", response.Header().Get("content-type"))
		assert.JSONEq(t, string(expected), response.Body.String())
		assert.Equal(t, http.StatusOK, response.Code)
	})

	t.Run("error", func(t *testing.T) {
		var (
			articlesRepository articles.MockArticlesRepository
			elementsRepository elements.MockElementsRepository
		)

		articlesRepository.On("GetMostViewed", uint(4)).Once().Return(nil, errors.New("an error has happened"))
		defer articlesRepository.AssertExpectations(t)

		useCase := home.NewUseCase(&articlesRepository, &elementsRepository)
		handler := NewHomeHandler(useCase)

		request := httptest.NewRequest(http.MethodGet, "/", nil)
		response := httptest.NewRecorder()

		handler.ServeHTTP(response, request)

		articlesRepository.AssertNotCalled(t, "GetAllPublished")
		articlesRepository.AssertNotCalled(t, "GetByUUIDs")
		elementsRepository.AssertNotCalled(t, "GetByVenues")

		assert.Len(t, response.Body.Bytes(), 0)
		assert.Equal(t, http.StatusInternalServerError, response.Code)
	})
}
