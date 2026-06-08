package article

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	getArticlesByAuthor "github.com/khanzadimahdi/testproject/application/article/getArticlesByAuthor"
	"github.com/khanzadimahdi/testproject/application/language/resolver"
	"github.com/khanzadimahdi/testproject/domain"
	"github.com/khanzadimahdi/testproject/domain/article"
	"github.com/khanzadimahdi/testproject/domain/language"
	"github.com/khanzadimahdi/testproject/domain/user"
	"github.com/khanzadimahdi/testproject/infrastructure/repository/mocks/articles"
	"github.com/khanzadimahdi/testproject/infrastructure/repository/mocks/users"
	"github.com/khanzadimahdi/testproject/infrastructure/validator"
)

const (
	testAuthorUUID = "01890d23-7b8e-7e4a-a9bd-1b8a52ad3a01"
	testUsername   = "author-username"
)

func TestIndexHandler(t *testing.T) {
	t.Parallel()

	t.Run("identity is a username", func(t *testing.T) {
		t.Parallel()

		var (
			articlesRepository articles.MockArticlesRepository
			userRepository     users.MockUsersRepository
			languageResolver   resolver.MockResolver
			requestValidator   validator.MockValidator
		)

		publishedAt, err := time.Parse(time.RFC3339, "2024-09-29T15:56:25Z")
		assert.NoError(t, err)

		u := user.User{
			UUID:     testAuthorUUID,
			Name:     "author-name",
			Avatar:   "author-avatar",
			Username: testUsername,
		}

		fetched := []article.Article{
			{
				UUID:        "article-uuid-1",
				Cover:       "article-cover-1",
				Video:       "article-video-1",
				Title:       "article-title-1",
				Excerpt:     "article-excerpt-1",
				PublishedAt: publishedAt,
				AuthorUUID:  testAuthorUUID,
			},
		}

		r := &getArticlesByAuthor.Request{
			Page:     1,
			Username: testUsername,
		}

		requestValidator.On("Validate", r).Once().Return(nil)
		defer requestValidator.AssertExpectations(t)

		languageResolver.On("DefaultCode").Once().Return("EN", nil)
		languageResolver.On("Resolve", "EN").Once().Return(language.Language{Code: "EN", Name: "English"}, nil)
		defer languageResolver.AssertExpectations(t)

		userRepository.On("GetOneByIdentity", testUsername).Once().Return(u, nil)
		defer userRepository.AssertExpectations(t)

		articlesRepository.On("CountPublishedByAuthor", testAuthorUUID, "EN").Once().Return(uint(len(fetched)), nil)
		articlesRepository.On("GetPublishedByAuthor", testAuthorUUID, "EN", uint(0), uint(10)).Once().Return(fetched, nil)
		articlesRepository.On("GetPublishedLanguages", "").Return([]language.Language{}, nil)
		defer articlesRepository.AssertExpectations(t)

		useCase := getArticlesByAuthor.NewUseCase(&articlesRepository, &userRepository, &languageResolver, &requestValidator)
		handler := NewIndexHandler(useCase)

		request := httptest.NewRequest(http.MethodGet, "/?page=1", nil)
		request.SetPathValue("identity", testUsername)

		response := httptest.NewRecorder()

		handler.ServeHTTP(response, request)

		expected, err := os.ReadFile("testdata/response-01.json")
		assert.NoError(t, err)

		assert.Equal(t, "application/json", response.Header().Get("content-type"))
		assert.JSONEq(t, string(expected), response.Body.String())
		assert.Equal(t, http.StatusOK, response.Code)
	})

	t.Run("identity is a uuid", func(t *testing.T) {
		t.Parallel()

		var (
			articlesRepository articles.MockArticlesRepository
			userRepository     users.MockUsersRepository
			languageResolver   resolver.MockResolver
			requestValidator   validator.MockValidator
		)

		u := user.User{UUID: testAuthorUUID, Username: testUsername}

		r := &getArticlesByAuthor.Request{
			Page:       1,
			AuthorUUID: testAuthorUUID,
		}

		requestValidator.On("Validate", r).Once().Return(nil)
		defer requestValidator.AssertExpectations(t)

		languageResolver.On("DefaultCode").Once().Return("EN", nil)
		languageResolver.On("Resolve", "EN").Once().Return(language.Language{Code: "EN", Name: "English"}, nil)
		defer languageResolver.AssertExpectations(t)

		userRepository.On("GetOne", testAuthorUUID).Once().Return(u, nil)
		userRepository.AssertNotCalled(t, "GetOneByIdentity")
		defer userRepository.AssertExpectations(t)

		articlesRepository.On("CountPublishedByAuthor", testAuthorUUID, "EN").Once().Return(uint(0), nil)
		articlesRepository.On("GetPublishedByAuthor", testAuthorUUID, "EN", uint(0), uint(10)).Once().Return(nil, nil)
		defer articlesRepository.AssertExpectations(t)

		useCase := getArticlesByAuthor.NewUseCase(&articlesRepository, &userRepository, &languageResolver, &requestValidator)
		handler := NewIndexHandler(useCase)

		request := httptest.NewRequest(http.MethodGet, "/?page=1", nil)
		request.SetPathValue("identity", testAuthorUUID)

		response := httptest.NewRecorder()

		handler.ServeHTTP(response, request)

		assert.Equal(t, "application/json", response.Header().Get("content-type"))
		assert.Equal(t, http.StatusOK, response.Code)
	})

	t.Run("author not found", func(t *testing.T) {
		t.Parallel()

		var (
			articlesRepository articles.MockArticlesRepository
			userRepository     users.MockUsersRepository
			languageResolver   resolver.MockResolver
			requestValidator   validator.MockValidator
		)

		r := &getArticlesByAuthor.Request{
			Page:     1,
			Username: "ghost",
		}

		requestValidator.On("Validate", r).Once().Return(nil)
		defer requestValidator.AssertExpectations(t)

		languageResolver.On("DefaultCode").Once().Return("EN", nil)
		languageResolver.On("Resolve", "EN").Once().Return(language.Language{Code: "EN", Name: "English"}, nil)
		defer languageResolver.AssertExpectations(t)

		userRepository.On("GetOneByIdentity", "ghost").Once().Return(user.User{}, domain.ErrNotExists)
		defer userRepository.AssertExpectations(t)

		useCase := getArticlesByAuthor.NewUseCase(&articlesRepository, &userRepository, &languageResolver, &requestValidator)
		handler := NewIndexHandler(useCase)

		request := httptest.NewRequest(http.MethodGet, "/?page=1", nil)
		request.SetPathValue("identity", "ghost")

		response := httptest.NewRecorder()

		handler.ServeHTTP(response, request)

		articlesRepository.AssertNotCalled(t, "CountPublishedByAuthor")
		articlesRepository.AssertNotCalled(t, "GetPublishedByAuthor")

		assert.Len(t, response.Body.Bytes(), 0)
		assert.Equal(t, http.StatusNotFound, response.Code)
	})

	t.Run("error", func(t *testing.T) {
		t.Parallel()

		var (
			articlesRepository articles.MockArticlesRepository
			userRepository     users.MockUsersRepository
			languageResolver   resolver.MockResolver
			requestValidator   validator.MockValidator
		)

		r := &getArticlesByAuthor.Request{
			Page:     1,
			Username: testUsername,
		}

		requestValidator.On("Validate", r).Once().Return(nil)
		defer requestValidator.AssertExpectations(t)

		languageResolver.On("DefaultCode").Once().Return("EN", nil)
		languageResolver.On("Resolve", "EN").Once().Return(language.Language{Code: "EN", Name: "English"}, nil)
		defer languageResolver.AssertExpectations(t)

		userRepository.On("GetOneByIdentity", testUsername).Once().Return(user.User{}, errors.New("boom"))
		defer userRepository.AssertExpectations(t)

		useCase := getArticlesByAuthor.NewUseCase(&articlesRepository, &userRepository, &languageResolver, &requestValidator)
		handler := NewIndexHandler(useCase)

		request := httptest.NewRequest(http.MethodGet, "/?page=1", nil)
		request.SetPathValue("identity", testUsername)

		response := httptest.NewRecorder()

		handler.ServeHTTP(response, request)

		assert.Len(t, response.Body.Bytes(), 0)
		assert.Equal(t, http.StatusInternalServerError, response.Code)
	})
}
