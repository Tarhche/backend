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
	getarticle "github.com/khanzadimahdi/testproject/application/dashboard/article/getArticle"
	"github.com/khanzadimahdi/testproject/domain"
	"github.com/khanzadimahdi/testproject/domain/article"
	"github.com/khanzadimahdi/testproject/domain/permission"
	"github.com/khanzadimahdi/testproject/domain/user"
	"github.com/khanzadimahdi/testproject/infrastructure/repository/mocks/articles"
)

func TestShowHandler(t *testing.T) {
	t.Run("show an article", func(t *testing.T) {
		var (
			articleRepository articles.MockArticlesRepository
			authorizer        domain.MockAuthorizer

			publishedAt, _ = time.Parse(time.RFC3339, "2024-10-11T04:27:44Z")

			a = article.Article{
				Title:       "article-title-1",
				UUID:        "article-uuid-1",
				Body:        "body-1",
				Excerpt:     "excerpt-1",
				Tags:        []string{"tag-1", "tag-2"},
				PublishedAt: publishedAt,
			}

			u = user.User{
				UUID: "user-uuid",
			}
		)

		authorizer.On("Authorize", u.UUID, permission.ArticlesShow).Once().Return(true, nil)
		defer authorizer.AssertExpectations(t)

		articleRepository.On("GetOne", a.UUID).Return(a, nil)
		defer articleRepository.AssertExpectations(t)

		handler := NewShowHandler(getarticle.NewUseCase(&articleRepository), &authorizer)

		request := httptest.NewRequest(http.MethodGet, "/", nil)
		request = request.WithContext(auth.ToContext(request.Context(), &u))
		request.SetPathValue("uuid", a.UUID)
		response := httptest.NewRecorder()

		handler.ServeHTTP(response, request)

		expectedBody, err := os.ReadFile("testdata/show-article-response.json")
		assert.NoError(t, err)

		assert.Equal(t, "application/json", response.Header().Get("content-type"))
		assert.JSONEq(t, string(expectedBody), response.Body.String())
		assert.Equal(t, http.StatusOK, response.Code)
	})

	t.Run("unauthorized", func(t *testing.T) {
		var (
			articleRepository articles.MockArticlesRepository
			authorizer        domain.MockAuthorizer

			a = article.Article{
				UUID: "article-uuid-1",
			}

			u = user.User{
				UUID: "user-uuid",
			}
		)

		authorizer.On("Authorize", u.UUID, permission.ArticlesShow).Once().Return(false, nil)
		defer authorizer.AssertExpectations(t)

		handler := NewShowHandler(getarticle.NewUseCase(&articleRepository), &authorizer)

		request := httptest.NewRequest(http.MethodGet, "/", nil)
		request = request.WithContext(auth.ToContext(request.Context(), &u))
		request.SetPathValue("uuid", a.UUID)
		response := httptest.NewRecorder()

		handler.ServeHTTP(response, request)

		articleRepository.AssertNotCalled(t, "GetOne")

		assert.Len(t, response.Body.Bytes(), 0)
		assert.Equal(t, http.StatusUnauthorized, response.Code)
	})

	t.Run("not found", func(t *testing.T) {
		var (
			articleRepository articles.MockArticlesRepository
			authorizer        domain.MockAuthorizer

			a = article.Article{
				UUID: "article-uuid-1",
			}

			u = user.User{
				UUID: "user-uuid",
			}
		)

		authorizer.On("Authorize", u.UUID, permission.ArticlesShow).Once().Return(true, nil)
		defer authorizer.AssertExpectations(t)

		articleRepository.On("GetOne", a.UUID).Return(article.Article{}, domain.ErrNotExists)
		defer articleRepository.AssertExpectations(t)

		handler := NewShowHandler(getarticle.NewUseCase(&articleRepository), &authorizer)

		request := httptest.NewRequest(http.MethodGet, "/", nil)
		request = request.WithContext(auth.ToContext(request.Context(), &u))
		request.SetPathValue("uuid", a.UUID)
		response := httptest.NewRecorder()

		handler.ServeHTTP(response, request)

		assert.Len(t, response.Body.Bytes(), 0)
		assert.Equal(t, http.StatusNotFound, response.Code)
	})

	t.Run("error", func(t *testing.T) {
		var (
			articleRepository articles.MockArticlesRepository
			authorizer        domain.MockAuthorizer

			a = article.Article{
				UUID: "article-uuid-1",
			}

			u = user.User{
				UUID: "user-uuid",
			}
		)

		authorizer.On("Authorize", u.UUID, permission.ArticlesShow).Once().Return(false, errors.New("unexpected error"))
		defer authorizer.AssertExpectations(t)

		handler := NewShowHandler(getarticle.NewUseCase(&articleRepository), &authorizer)

		request := httptest.NewRequest(http.MethodGet, "/", nil)
		request = request.WithContext(auth.ToContext(request.Context(), &u))
		request.SetPathValue("uuid", a.UUID)
		response := httptest.NewRecorder()

		handler.ServeHTTP(response, request)

		articleRepository.AssertNotCalled(t, "GetOne")

		assert.Len(t, response.Body.Bytes(), 0)
		assert.Equal(t, http.StatusInternalServerError, response.Code)
	})
}
