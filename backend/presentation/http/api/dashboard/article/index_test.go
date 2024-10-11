package article

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/khanzadimahdi/testproject/application/auth"
	getarticles "github.com/khanzadimahdi/testproject/application/dashboard/article/getArticles"
	"github.com/khanzadimahdi/testproject/domain"
	"github.com/khanzadimahdi/testproject/domain/article"
	"github.com/khanzadimahdi/testproject/domain/permission"
	"github.com/khanzadimahdi/testproject/domain/user"
	"github.com/khanzadimahdi/testproject/infrastructure/repository/mocks/articles"
)

func TestIndexHandler(t *testing.T) {
	t.Run("show articles", func(t *testing.T) {
		var (
			articleRepository articles.MockArticlesRepository
			authorizer        domain.MockAuthorizer

			publishedAt, _ = time.Parse(time.RFC3339, "2024-10-11T04:27:44Z")

			a = []article.Article{
				{
					Title:   "article-title-1",
					UUID:    "article-uuid-1",
					Body:    "body-1",
					Excerpt: "excerpt-1",
				},
				{
					UUID: "article-uuid-2",
					Tags: []string{"tag-1", "tag-2"},
				},
				{
					UUID:        "article-uuid-3",
					Tags:        []string{},
					PublishedAt: publishedAt,
				},
			}

			u = user.User{
				UUID: "user-uuid",
			}
		)

		authorizer.On("Authorize", u.UUID, permission.ArticlesIndex).Once().Return(true, nil)
		defer authorizer.AssertExpectations(t)

		articleRepository.On("Count").Once().Return(uint(len(a)), nil)
		articleRepository.On("GetAll", uint(0), uint(10)).Return(a, nil)
		defer articleRepository.AssertExpectations(t)

		handler := NewIndexHandler(getarticles.NewUseCase(&articleRepository), &authorizer)

		request := httptest.NewRequest(http.MethodGet, "/?page=1", nil)
		request = request.WithContext(auth.ToContext(request.Context(), &u))
		response := httptest.NewRecorder()

		handler.ServeHTTP(response, request)

		expectedBody, err := os.ReadFile("testdata/show-articles-response.json")
		assert.NoError(t, err)

		assert.Equal(t, "application/json", response.Header().Get("content-type"))
		assert.JSONEq(t, string(expectedBody), response.Body.String())
		assert.Equal(t, http.StatusOK, response.Code)
	})

	t.Run("no data", func(t *testing.T) {
		var (
			articleRepository articles.MockArticlesRepository
			authorizer        domain.MockAuthorizer

			u = user.User{
				UUID: "user-uuid",
			}
		)

		authorizer.On("Authorize", u.UUID, permission.ArticlesIndex).Once().Return(true, nil)
		defer authorizer.AssertExpectations(t)

		articleRepository.On("Count").Once().Return(uint(0), nil)
		articleRepository.On("GetAll", uint(0), uint(10)).Return(nil, nil)
		defer articleRepository.AssertExpectations(t)

		handler := NewIndexHandler(getarticles.NewUseCase(&articleRepository), &authorizer)

		request := httptest.NewRequest(http.MethodGet, "/?page=1", nil)
		request = request.WithContext(auth.ToContext(request.Context(), &u))
		response := httptest.NewRecorder()

		handler.ServeHTTP(response, request)

		expectedBody, err := os.ReadFile("testdata/show-articles-no-data-response.json")
		assert.NoError(t, err)

		assert.Equal(t, "application/json", response.Header().Get("content-type"))
		assert.JSONEq(t, string(expectedBody), response.Body.String())
		assert.Equal(t, http.StatusOK, response.Code)
	})

	t.Run("unauthorized", func(t *testing.T) {
		var (
			articleRepository articles.MockArticlesRepository
			authorizer        domain.MockAuthorizer

			u = user.User{
				UUID: "user-uuid",
			}
		)

		authorizer.On("Authorize", u.UUID, permission.ArticlesIndex).Once().Return(false, nil)
		defer authorizer.AssertExpectations(t)

		handler := NewIndexHandler(getarticles.NewUseCase(&articleRepository), &authorizer)

		request := httptest.NewRequest(http.MethodGet, "/?page=1", nil)
		request = request.WithContext(auth.ToContext(request.Context(), &u))
		response := httptest.NewRecorder()

		handler.ServeHTTP(response, request)

		articleRepository.AssertNotCalled(t, "Count")
		articleRepository.AssertNotCalled(t, "GetAll")

		assert.Len(t, response.Body.Bytes(), 0)
		assert.Equal(t, http.StatusUnauthorized, response.Code)
	})

	t.Run("error", func(t *testing.T) {
		var (
			articleRepository articles.MockArticlesRepository
			authorizer        domain.MockAuthorizer

			u = user.User{
				UUID: "user-uuid",
			}
		)

		authorizer.On("Authorize", u.UUID, permission.ArticlesIndex).Once().Return(false, errors.New("unexpected error"))
		defer authorizer.AssertExpectations(t)

		handler := NewIndexHandler(getarticles.NewUseCase(&articleRepository), &authorizer)

		request := httptest.NewRequest(http.MethodGet, "/?page=1", nil)
		request = request.WithContext(auth.ToContext(request.Context(), &u))
		response := httptest.NewRecorder()

		handler.ServeHTTP(response, request)

		articleRepository.AssertNotCalled(t, "Count")
		articleRepository.AssertNotCalled(t, "GetAll")

		assert.Len(t, response.Body.Bytes(), 0)
		assert.Equal(t, http.StatusInternalServerError, response.Code)
	})
}
