package article

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	getarticles "github.com/khanzadimahdi/testproject/application/article/getArticles"
	"github.com/khanzadimahdi/testproject/domain/article"
	"github.com/khanzadimahdi/testproject/domain/author"
	"github.com/khanzadimahdi/testproject/infrastructure/repository/mocks/articles"
)

func TestIndexHandler(t *testing.T) {
	t.Parallel()

	t.Run("get list of articles", func(t *testing.T) {
		t.Parallel()

		var (
			articlesRepository articles.MockArticlesRepository
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

		articlesRepository.On("CountPublished").Once().Return(uint(len(articles)), nil)
		articlesRepository.On("GetAllPublished", uint(0), uint(10)).Once().Return(articles, nil)
		defer articlesRepository.AssertExpectations(t)

		handler := NewIndexHandler(getarticles.NewUseCase(&articlesRepository))

		request := httptest.NewRequest(http.MethodGet, "/?page=1", nil)
		response := httptest.NewRecorder()

		handler.ServeHTTP(response, request)

		expected, err := os.ReadFile("testdata/response-index-response.json")
		assert.NoError(t, err)

		assert.Equal(t, "application/json", response.Header().Get("content-type"))
		assert.JSONEq(t, string(expected), response.Body.String())
		assert.Equal(t, http.StatusOK, response.Code)
	})

	t.Run("get empty list of articles", func(t *testing.T) {
		t.Parallel()

		var (
			articlesRepository articles.MockArticlesRepository
		)

		articlesRepository.On("CountPublished").Once().Return(uint(0), nil)
		articlesRepository.On("GetAllPublished", uint(0), uint(10)).Once().Return(nil, nil)
		defer articlesRepository.AssertExpectations(t)

		handler := NewIndexHandler(getarticles.NewUseCase(&articlesRepository))

		request := httptest.NewRequest(http.MethodGet, "/?page=1", nil)
		response := httptest.NewRecorder()

		handler.ServeHTTP(response, request)

		expected, err := os.ReadFile("testdata/response-index-no-data-response.json")
		assert.NoError(t, err)

		assert.Equal(t, "application/json", response.Header().Get("content-type"))
		assert.JSONEq(t, string(expected), response.Body.String())
		assert.Equal(t, http.StatusOK, response.Code)
	})

	t.Run("error", func(t *testing.T) {
		t.Parallel()

		var (
			articlesRepository articles.MockArticlesRepository
		)

		articlesRepository.On("CountPublished").Once().Return(uint(0), errors.New("something faulty has happened"))
		defer articlesRepository.AssertExpectations(t)

		handler := NewIndexHandler(getarticles.NewUseCase(&articlesRepository))

		request := httptest.NewRequest(http.MethodGet, "/?page=1", nil)
		response := httptest.NewRecorder()

		handler.ServeHTTP(response, request)

		articlesRepository.AssertNotCalled(t, "GetAllPublished")

		assert.Len(t, response.Body.Bytes(), 0)
		assert.Equal(t, http.StatusInternalServerError, response.Code)
	})
}
