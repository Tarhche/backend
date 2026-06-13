package article

import (
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	getarticle "github.com/khanzadimahdi/testproject/application/dashboard/article/getArticle"
	"github.com/khanzadimahdi/testproject/domain"
	"github.com/khanzadimahdi/testproject/domain/article"
	"github.com/khanzadimahdi/testproject/domain/user"
	"github.com/khanzadimahdi/testproject/infrastructure/repository/mocks/articles"
	"github.com/khanzadimahdi/testproject/infrastructure/repository/mocks/users"
)

func TestShowHandler(t *testing.T) {
	t.Parallel()

	t.Run("show an article", func(t *testing.T) {
		t.Parallel()

		var (
			articleRepository articles.MockArticlesRepository
			userRepository    users.MockUsersRepository

			publishedAt, _ = time.Parse(time.RFC3339, "2024-10-11T04:27:44Z")

			a = article.Article{
				Title:           "article-title-1",
				UUID:            "article-uuid-1",
				CorrelationUUID: "correlation-uuid-1",
				LanguageCode:    "EN",
				Body:            "body-1",
				Excerpt:         "excerpt-1",
				Tags:            []string{"tag-1", "tag-2"},
				PublishedAt:     publishedAt,
			}
		)

		articleRepository.On("GetByCorrelationUUIDAndLanguage", a.CorrelationUUID, a.LanguageCode).Return(a, nil)
		defer articleRepository.AssertExpectations(t)

		userRepository.On("GetOne", "").Return(user.User{}, nil)
		defer userRepository.AssertExpectations(t)

		handler := NewShowHandler(getarticle.NewUseCase(&articleRepository, &userRepository))

		request := httptest.NewRequest(http.MethodGet, "/", nil)
		request.SetPathValue("correlationUUID", a.CorrelationUUID)
		request.SetPathValue("language_code", a.LanguageCode)
		response := httptest.NewRecorder()

		handler.ServeHTTP(response, request)

		expectedBody, err := os.ReadFile("testdata/show-article-response.json")
		assert.NoError(t, err)

		assert.Equal(t, "application/json", response.Header().Get("content-type"))
		assert.JSONEq(t, string(expectedBody), response.Body.String())
		assert.Equal(t, http.StatusOK, response.Code)
	})

	t.Run("not found", func(t *testing.T) {
		t.Parallel()

		var (
			articleRepository articles.MockArticlesRepository
			userRepository    users.MockUsersRepository

			a = article.Article{
				CorrelationUUID: "correlation-uuid-1",
				LanguageCode:    "EN",
			}
		)

		articleRepository.On("GetByCorrelationUUIDAndLanguage", a.CorrelationUUID, a.LanguageCode).Return(article.Article{}, domain.ErrNotExists)
		defer articleRepository.AssertExpectations(t)

		handler := NewShowHandler(getarticle.NewUseCase(&articleRepository, &userRepository))

		request := httptest.NewRequest(http.MethodGet, "/", nil)
		request.SetPathValue("correlationUUID", a.CorrelationUUID)
		request.SetPathValue("language_code", a.LanguageCode)
		response := httptest.NewRecorder()

		handler.ServeHTTP(response, request)

		userRepository.AssertNotCalled(t, "GetOne")

		assert.Len(t, response.Body.Bytes(), 0)
		assert.Equal(t, http.StatusNotFound, response.Code)
	})
}
