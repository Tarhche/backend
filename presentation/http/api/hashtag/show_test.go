package hashtag

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	getArticlesByHashtag "github.com/khanzadimahdi/testproject/application/article/getArticlesByHashtag"
	"github.com/khanzadimahdi/testproject/domain"
	"github.com/khanzadimahdi/testproject/domain/article"
	"github.com/khanzadimahdi/testproject/domain/author"
	"github.com/khanzadimahdi/testproject/infrastructure/repository/mocks/articles"
	"github.com/khanzadimahdi/testproject/infrastructure/validator"
)

func TestShowHandler(t *testing.T) {
	t.Parallel()

	t.Run("show home data", func(t *testing.T) {
		t.Parallel()

		var (
			articlesRepository articles.MockArticlesRepository
			requestValidator   validator.MockValidator
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

		hashtag := "a-test-hashtag"

		r := &getArticlesByHashtag.Request{
			Page:    1,
			Hashtag: hashtag,
		}

		articlesRepository.On("GetByHashtag", []string{hashtag}, uint(0), uint(10)).Once().Return(articles, nil)
		defer articlesRepository.AssertExpectations(t)

		requestValidator.On("Validate", r).Once().Return(nil)
		defer requestValidator.AssertExpectations(t)

		useCase := getArticlesByHashtag.NewUseCase(&articlesRepository, &requestValidator)
		handler := NewShowHandler(useCase)

		request := httptest.NewRequest(http.MethodGet, "/", nil)
		request.SetPathValue("hashtag", hashtag)

		response := httptest.NewRecorder()

		handler.ServeHTTP(response, request)

		expected, err := os.ReadFile("testdata/response-01.txt")
		assert.NoError(t, err)

		assert.Equal(t, "application/json", response.Header().Get("content-type"))
		assert.JSONEq(t, string(expected), response.Body.String())
		assert.Equal(t, http.StatusOK, response.Code)
	})

	t.Run("validation failed", func(t *testing.T) {
		t.Parallel()

		var (
			articlesRepository articles.MockArticlesRepository
			requestValidator   validator.MockValidator
		)

		r := &getArticlesByHashtag.Request{
			Page: 1,
		}

		validationErrors := domain.ValidationErrors{
			"hashtag": "this field is required",
		}

		requestValidator.On("Validate", r).Once().Return(validationErrors)
		defer requestValidator.AssertExpectations(t)

		useCase := getArticlesByHashtag.NewUseCase(&articlesRepository, &requestValidator)
		handler := NewShowHandler(useCase)

		request := httptest.NewRequest(http.MethodGet, "/", nil)

		response := httptest.NewRecorder()

		handler.ServeHTTP(response, request)

		expected, err := os.ReadFile("testdata/response-validation-failed.txt")
		assert.NoError(t, err)

		articlesRepository.AssertNotCalled(t, "GetByHashtag")

		assert.Equal(t, "application/json", response.Header().Get("content-type"))
		assert.JSONEq(t, string(expected), response.Body.String())
		assert.Equal(t, http.StatusBadRequest, response.Code)
	})

	t.Run("no data", func(t *testing.T) {
		t.Parallel()

		var (
			articlesRepository articles.MockArticlesRepository
			requestValidator   validator.MockValidator
		)

		hashtag := "a-test-hashtag"

		r := &getArticlesByHashtag.Request{
			Page:    1,
			Hashtag: hashtag,
		}

		articlesRepository.On("GetByHashtag", []string{hashtag}, uint(0), uint(10)).Once().Return(nil, nil)
		defer articlesRepository.AssertExpectations(t)

		requestValidator.On("Validate", r).Once().Return(nil)
		defer requestValidator.AssertExpectations(t)

		useCase := getArticlesByHashtag.NewUseCase(&articlesRepository, &requestValidator)
		handler := NewShowHandler(useCase)

		request := httptest.NewRequest(http.MethodGet, "/", nil)
		request.SetPathValue("hashtag", hashtag)

		response := httptest.NewRecorder()

		handler.ServeHTTP(response, request)

		expected, err := os.ReadFile("testdata/response-02.txt")
		assert.NoError(t, err)

		assert.Equal(t, "application/json", response.Header().Get("content-type"))
		assert.JSONEq(t, string(expected), response.Body.String())
		assert.Equal(t, http.StatusOK, response.Code)
	})

	t.Run("error", func(t *testing.T) {
		t.Parallel()

		var (
			articlesRepository articles.MockArticlesRepository
			requestValidator   validator.MockValidator
		)

		hashtag := "a-test-hashtag"

		r := &getArticlesByHashtag.Request{
			Page:    1,
			Hashtag: hashtag,
		}

		articlesRepository.On("GetByHashtag", []string{hashtag}, uint(0), uint(10)).Once().Return(nil, errors.New("some error happened"))
		defer articlesRepository.AssertExpectations(t)

		requestValidator.On("Validate", r).Once().Return(nil)
		defer requestValidator.AssertExpectations(t)

		useCase := getArticlesByHashtag.NewUseCase(&articlesRepository, &requestValidator)
		handler := NewShowHandler(useCase)

		request := httptest.NewRequest(http.MethodGet, "/", nil)
		request.SetPathValue("hashtag", hashtag)

		response := httptest.NewRecorder()

		handler.ServeHTTP(response, request)

		assert.Len(t, response.Body.Bytes(), 0)
		assert.Equal(t, http.StatusInternalServerError, response.Code)
	})
}
