package article

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/khanzadimahdi/testproject/application/auth"
	deletearticle "github.com/khanzadimahdi/testproject/application/dashboard/article/deleteArticle"
	"github.com/khanzadimahdi/testproject/domain"
	"github.com/khanzadimahdi/testproject/domain/permission"
	"github.com/khanzadimahdi/testproject/domain/user"
	"github.com/khanzadimahdi/testproject/infrastructure/repository/mocks/articles"
)

func TestDeleteHandler(t *testing.T) {
	t.Run("delete an article", func(t *testing.T) {
		var (
			articleRepository articles.MockArticlesRepository
			authorizer        domain.MockAuthorizer

			r = deletearticle.Request{ArticleUUID: "article-uuid"}
			u = user.User{
				UUID: "user-uuid",
			}
		)

		authorizer.On("Authorize", u.UUID, permission.ArticlesDelete).Once().Return(true, nil)
		defer authorizer.AssertExpectations(t)

		articleRepository.On("Delete", r.ArticleUUID).Return(nil)
		defer articleRepository.AssertExpectations(t)

		handler := NewDeleteHandler(deletearticle.NewUseCase(&articleRepository), &authorizer)

		request := httptest.NewRequest(http.MethodPost, "/", nil)
		request = request.WithContext(auth.ToContext(request.Context(), &u))
		request.SetPathValue("uuid", r.ArticleUUID)
		response := httptest.NewRecorder()

		handler.ServeHTTP(response, request)

		assert.Len(t, response.Body.Bytes(), 0)
		assert.Equal(t, http.StatusNoContent, response.Code)
	})

	t.Run("unauthorized", func(t *testing.T) {
		var (
			articleRepository articles.MockArticlesRepository
			authorizer        domain.MockAuthorizer

			r = deletearticle.Request{ArticleUUID: "article-uuid"}
			u = user.User{
				UUID: "user-uuid",
			}
		)

		authorizer.On("Authorize", u.UUID, permission.ArticlesDelete).Once().Return(false, nil)
		defer authorizer.AssertExpectations(t)

		handler := NewDeleteHandler(deletearticle.NewUseCase(&articleRepository), &authorizer)

		request := httptest.NewRequest(http.MethodPost, "/", nil)
		request = request.WithContext(auth.ToContext(request.Context(), &u))
		request.SetPathValue("uuid", r.ArticleUUID)
		response := httptest.NewRecorder()

		handler.ServeHTTP(response, request)

		articleRepository.AssertNotCalled(t, "Delete")

		assert.Len(t, response.Body.Bytes(), 0)
		assert.Equal(t, http.StatusUnauthorized, response.Code)
	})

	t.Run("error", func(t *testing.T) {
		var (
			articleRepository articles.MockArticlesRepository
			authorizer        domain.MockAuthorizer

			r = deletearticle.Request{ArticleUUID: "article-uuid"}
			u = user.User{
				UUID: "user-uuid",
			}
		)

		authorizer.On("Authorize", u.UUID, permission.ArticlesDelete).Once().Return(false, errors.New("unexpected error"))
		defer authorizer.AssertExpectations(t)

		handler := NewDeleteHandler(deletearticle.NewUseCase(&articleRepository), &authorizer)

		request := httptest.NewRequest(http.MethodPost, "/", nil)
		request = request.WithContext(auth.ToContext(request.Context(), &u))
		request.SetPathValue("uuid", r.ArticleUUID)
		response := httptest.NewRecorder()

		handler.ServeHTTP(response, request)

		articleRepository.AssertNotCalled(t, "Delete")

		assert.Len(t, response.Body.Bytes(), 0)
		assert.Equal(t, http.StatusInternalServerError, response.Code)
	})
}
