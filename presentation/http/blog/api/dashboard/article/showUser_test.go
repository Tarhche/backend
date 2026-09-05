package article

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"

	"github.com/khanzadimahdi/testproject/application/auth"
	getuserarticle "github.com/khanzadimahdi/testproject/application/dashboard/article/getUserArticle"
	"github.com/khanzadimahdi/testproject/domain"
	"github.com/khanzadimahdi/testproject/domain/article"
	"github.com/khanzadimahdi/testproject/domain/user"
	"github.com/khanzadimahdi/testproject/infrastructure/repository/mocks/articles"
	"github.com/khanzadimahdi/testproject/infrastructure/repository/mocks/users"
)

func TestShowUserHandler(t *testing.T) {
	t.Parallel()

	t.Run("shows an article the person wrote", func(t *testing.T) {
		t.Parallel()

		var (
			articleRepository articles.MockArticlesRepository
			userRepository    users.MockUsersRepository

			u = user.User{UUID: "author-uuid", Name: "author-name"}
			a = article.Article{
				UUID:            "article-uuid",
				CorrelationUUID: "correlation-uuid",
				LanguageCode:    "EN",
				AuthorUUID:      "author-uuid",
			}
		)

		articleRepository.On("GetByCorrelationUUIDAndLanguageAndAuthor", mock.Anything, a.CorrelationUUID, a.LanguageCode, u.UUID).Once().Return(a, nil)
		defer articleRepository.AssertExpectations(t)

		userRepository.On("GetOne", mock.Anything, u.UUID).Once().Return(u, nil)
		defer userRepository.AssertExpectations(t)

		handler := NewShowUserHandler(getuserarticle.NewUseCase(&articleRepository, &userRepository))

		request := httptest.NewRequest(http.MethodGet, "/", nil)
		request = request.WithContext(auth.ToContext(request.Context(), &u))
		request.SetPathValue("correlationUUID", a.CorrelationUUID)
		request.SetPathValue("language_code", a.LanguageCode)
		response := httptest.NewRecorder()

		handler.ServeHTTP(response, request)

		assert.Equal(t, http.StatusOK, response.Code)
		assert.Equal(t, "application/json", response.Header().Get("content-type"))
	})

	t.Run("an article somebody else wrote is not found", func(t *testing.T) {
		t.Parallel()

		var (
			articleRepository articles.MockArticlesRepository
			userRepository    users.MockUsersRepository

			u = user.User{UUID: "author-uuid"}
		)

		articleRepository.On("GetByCorrelationUUIDAndLanguageAndAuthor", mock.Anything, "correlation-uuid", "EN", u.UUID).
			Once().Return(article.Article{}, domain.ErrNotExists)
		defer articleRepository.AssertExpectations(t)

		handler := NewShowUserHandler(getuserarticle.NewUseCase(&articleRepository, &userRepository))

		request := httptest.NewRequest(http.MethodGet, "/", nil)
		request = request.WithContext(auth.ToContext(request.Context(), &u))
		request.SetPathValue("correlationUUID", "correlation-uuid")
		request.SetPathValue("language_code", "EN")
		response := httptest.NewRecorder()

		handler.ServeHTTP(response, request)

		assert.Equal(t, http.StatusNotFound, response.Code)
		userRepository.AssertNotCalled(t, "GetOne", mock.Anything, mock.Anything)
	})
}
