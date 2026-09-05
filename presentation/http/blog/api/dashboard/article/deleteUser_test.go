package article

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"

	"github.com/khanzadimahdi/testproject/application/auth"
	deleteuserarticle "github.com/khanzadimahdi/testproject/application/dashboard/article/deleteUserArticle"
	"github.com/khanzadimahdi/testproject/domain"
	"github.com/khanzadimahdi/testproject/domain/article"
	"github.com/khanzadimahdi/testproject/domain/user"
	"github.com/khanzadimahdi/testproject/infrastructure/repository/mocks/articles"
)

func TestDeleteUserHandler(t *testing.T) {
	t.Parallel()

	t.Run("deletes an article the person wrote", func(t *testing.T) {
		t.Parallel()

		var (
			articleRepository articles.MockArticlesRepository

			u = user.User{UUID: "author-uuid"}
			a = article.Article{
				UUID:            "article-uuid",
				CorrelationUUID: "correlation-uuid",
				LanguageCode:    "EN",
				AuthorUUID:      u.UUID,
			}
		)

		articleRepository.On("GetByCorrelationUUIDAndLanguageAndAuthor", mock.Anything, a.CorrelationUUID, a.LanguageCode, u.UUID).Once().Return(a, nil)
		articleRepository.On("DeleteByCorrelationUUIDAndLanguageAndAuthor", mock.Anything, a.CorrelationUUID, a.LanguageCode, u.UUID).Once().Return(nil)
		defer articleRepository.AssertExpectations(t)

		handler := NewDeleteUserHandler(deleteuserarticle.NewUseCase(&articleRepository))

		request := httptest.NewRequest(http.MethodDelete, "/", nil)
		request = request.WithContext(auth.ToContext(request.Context(), &u))
		request.SetPathValue("correlationUUID", a.CorrelationUUID)
		request.SetPathValue("language_code", a.LanguageCode)
		response := httptest.NewRecorder()

		handler.ServeHTTP(response, request)

		assert.Equal(t, http.StatusNoContent, response.Code)
	})

	t.Run("an article somebody else wrote is not found", func(t *testing.T) {
		t.Parallel()

		var (
			articleRepository articles.MockArticlesRepository

			u = user.User{UUID: "author-uuid"}
		)

		articleRepository.On("GetByCorrelationUUIDAndLanguageAndAuthor", mock.Anything, "correlation-uuid", "EN", u.UUID).
			Once().Return(article.Article{}, domain.ErrNotExists)
		defer articleRepository.AssertExpectations(t)

		handler := NewDeleteUserHandler(deleteuserarticle.NewUseCase(&articleRepository))

		request := httptest.NewRequest(http.MethodDelete, "/", nil)
		request = request.WithContext(auth.ToContext(request.Context(), &u))
		request.SetPathValue("correlationUUID", "correlation-uuid")
		request.SetPathValue("language_code", "EN")
		response := httptest.NewRecorder()

		handler.ServeHTTP(response, request)

		assert.Equal(t, http.StatusNotFound, response.Code)
	})
}
