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
	"github.com/khanzadimahdi/testproject/application/language/resolver"
	"github.com/khanzadimahdi/testproject/domain/article"
	"github.com/khanzadimahdi/testproject/domain/language"
	"github.com/khanzadimahdi/testproject/domain/user"
	"github.com/khanzadimahdi/testproject/infrastructure/repository/mocks/articles"
	"github.com/khanzadimahdi/testproject/infrastructure/repository/mocks/users"
)

func TestIndexHandler(t *testing.T) {
	t.Parallel()

	t.Run("get list of articles", func(t *testing.T) {
		t.Parallel()

		var (
			articlesRepository articles.MockArticlesRepository
			userRepository     users.MockUsersRepository
			languageResolver   resolver.MockResolver
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
				AuthorUUID:  "author-uuid-1",
				Tags:        []string{"tag-1", "tag-2", "tag-3"},
				ViewCount:   123,
			},
			{
				UUID:       "article-uuid-2",
				Cover:      "article-cover-2",
				Title:      "article-title-2",
				Excerpt:    "article-excerpt-2",
				Body:       "article-body-2",
				AuthorUUID: "author-uuid-1",
			},
			{
				UUID:       "article-uuid-3",
				Cover:      "article-cover-3",
				Title:      "article-title-3",
				Excerpt:    "article-excerpt-3",
				Body:       "article-body-3",
				AuthorUUID: "author-uuid-2",
			},
		}

		users := []user.User{
			{UUID: "author-uuid-1", Name: "author-name", Avatar: "author-avatar", Username: "author-username-1"},
			{UUID: "author-uuid-2", Name: "author-name", Avatar: "author-avatar", Username: "author-username-2"},
		}

		articlesRepository.On("CountPublished", "EN").Once().Return(uint(len(articles)), nil)
		articlesRepository.On("GetAllPublished", "EN", uint(0), uint(10)).Once().Return(articles, nil)
		defer articlesRepository.AssertExpectations(t)

		userRepository.On("GetByUUIDs", []string{"author-uuid-1", "author-uuid-1", "author-uuid-2"}).Once().Return(users, nil)
		defer userRepository.AssertExpectations(t)

		articlesRepository.On("GetPublishedLanguages", "").Return([]language.Language{}, nil)

		languageResolver.On("DefaultCode").Once().Return("EN", nil)
		languageResolver.On("Resolve", "EN").Once().Return(language.Language{Code: "EN", Name: "English"}, nil)
		defer languageResolver.AssertExpectations(t)

		handler := NewIndexHandler(getarticles.NewUseCase(&articlesRepository, &userRepository, &languageResolver))

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
			userRepository     users.MockUsersRepository
			languageResolver   resolver.MockResolver
		)

		articlesRepository.On("CountPublished", "EN").Once().Return(uint(0), nil)
		articlesRepository.On("GetAllPublished", "EN", uint(0), uint(10)).Once().Return(nil, nil)
		defer articlesRepository.AssertExpectations(t)

		userRepository.On("GetByUUIDs", []string{}).Once().Return([]user.User{}, nil)
		defer userRepository.AssertExpectations(t)

		languageResolver.On("DefaultCode").Once().Return("EN", nil)
		languageResolver.On("Resolve", "EN").Once().Return(language.Language{Code: "EN", Name: "English"}, nil)
		defer languageResolver.AssertExpectations(t)

		handler := NewIndexHandler(getarticles.NewUseCase(&articlesRepository, &userRepository, &languageResolver))

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
			userRepository     users.MockUsersRepository
			languageResolver   resolver.MockResolver
		)

		articlesRepository.On("CountPublished", "EN").Once().Return(uint(0), errors.New("something faulty has happened"))
		defer articlesRepository.AssertExpectations(t)

		languageResolver.On("DefaultCode").Once().Return("EN", nil)
		languageResolver.On("Resolve", "EN").Once().Return(language.Language{Code: "EN", Name: "English"}, nil)
		defer languageResolver.AssertExpectations(t)

		handler := NewIndexHandler(getarticles.NewUseCase(&articlesRepository, &userRepository, &languageResolver))

		request := httptest.NewRequest(http.MethodGet, "/?page=1", nil)
		response := httptest.NewRecorder()

		handler.ServeHTTP(response, request)

		articlesRepository.AssertNotCalled(t, "GetAllPublished")

		assert.Len(t, response.Body.Bytes(), 0)
		assert.Equal(t, http.StatusInternalServerError, response.Code)
	})
}
