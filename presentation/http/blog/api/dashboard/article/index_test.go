package article

import (
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"

	getarticles "github.com/khanzadimahdi/testproject/application/dashboard/article/getArticles"
	"github.com/khanzadimahdi/testproject/domain/article"
	"github.com/khanzadimahdi/testproject/domain/language"
	"github.com/khanzadimahdi/testproject/domain/user"
	"github.com/khanzadimahdi/testproject/infrastructure/repository/mocks/articles"
	"github.com/khanzadimahdi/testproject/infrastructure/repository/mocks/languages"
	"github.com/khanzadimahdi/testproject/infrastructure/repository/mocks/users"
)

func TestIndexHandler(t *testing.T) {
	t.Parallel()

	t.Run("show articles", func(t *testing.T) {
		t.Parallel()

		var (
			articleRepository  articles.MockArticlesRepository
			userRepository     users.MockUsersRepository
			languageRepository languages.MockLanguagesRepository

			correlationUUIDs = []string{"correlation-uuid-1", "correlation-uuid-2"}

			a = []article.Article{
				{UUID: "article-uuid-1", CorrelationUUID: "correlation-uuid-1", LanguageCode: "EN", Title: "title-1-en", AuthorUUID: "author-1"},
				{UUID: "article-uuid-2", CorrelationUUID: "correlation-uuid-1", LanguageCode: "FA", Title: "title-1-fa", AuthorUUID: "author-1"},
				{UUID: "article-uuid-3", CorrelationUUID: "correlation-uuid-2", LanguageCode: "EN", Title: "title-2-en", AuthorUUID: "author-2"},
			}
			u = []user.User{
				{UUID: "author-1", Name: "Author One", Avatar: "a1.png", Username: "author_one"},
				{UUID: "author-2", Name: "Author Two", Avatar: "a2.png", Username: "author_two"},
			}
			l = []language.Language{
				{Code: "EN", Name: "English"},
				{Code: "FA", Name: "Persian"},
			}
		)

		articleRepository.On("CountByCorrelation", mock.Anything).Once().Return(uint(len(correlationUUIDs)), nil)
		articleRepository.On("GetCorrelationUUIDs", mock.Anything, uint(0), uint(20)).Once().Return(correlationUUIDs, nil)
		articleRepository.On("GetByCorrelationUUIDs", mock.Anything, correlationUUIDs, "").Once().Return(a, nil)
		defer articleRepository.AssertExpectations(t)

		userRepository.On("GetByUUIDs", mock.Anything, []string{"author-1", "author-1", "author-2"}).Once().Return(u, nil)
		defer userRepository.AssertExpectations(t)

		languageRepository.On("GetByCodes", mock.Anything, []string{"EN", "FA", "EN"}).Once().Return(l, nil)
		defer languageRepository.AssertExpectations(t)

		handler := NewIndexHandler(getarticles.NewUseCase(&articleRepository, &userRepository, &languageRepository))

		request := httptest.NewRequest(http.MethodGet, "/?page=1", nil)
		response := httptest.NewRecorder()

		handler.ServeHTTP(response, request)

		expectedBody, err := os.ReadFile("testdata/show-articles-response.json")
		assert.NoError(t, err)

		assert.Equal(t, "application/json", response.Header().Get("content-type"))
		assert.JSONEq(t, string(expectedBody), response.Body.String())
		assert.Equal(t, http.StatusOK, response.Code)
	})

	t.Run("no data", func(t *testing.T) {
		t.Parallel()

		var (
			articleRepository  articles.MockArticlesRepository
			userRepository     users.MockUsersRepository
			languageRepository languages.MockLanguagesRepository
		)

		articleRepository.On("CountByCorrelation", mock.Anything).Once().Return(uint(0), nil)
		articleRepository.On("GetCorrelationUUIDs", mock.Anything, uint(0), uint(20)).Once().Return([]string{}, nil)
		defer articleRepository.AssertExpectations(t)

		handler := NewIndexHandler(getarticles.NewUseCase(&articleRepository, &userRepository, &languageRepository))

		request := httptest.NewRequest(http.MethodGet, "/?page=1", nil)
		response := httptest.NewRecorder()

		handler.ServeHTTP(response, request)

		expectedBody, err := os.ReadFile("testdata/show-articles-no-data-response.json")
		assert.NoError(t, err)

		assert.Equal(t, "application/json", response.Header().Get("content-type"))
		assert.JSONEq(t, string(expectedBody), response.Body.String())
		assert.Equal(t, http.StatusOK, response.Code)
	})
}
