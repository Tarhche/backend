package article

import (
	"bytes"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/khanzadimahdi/testproject/application/auth"
	updatearticle "github.com/khanzadimahdi/testproject/application/dashboard/article/updateArticle"
	"github.com/khanzadimahdi/testproject/domain"
	"github.com/khanzadimahdi/testproject/domain/article"
	"github.com/khanzadimahdi/testproject/domain/user"
	"github.com/khanzadimahdi/testproject/infrastructure/repository/mocks/articles"
	"github.com/khanzadimahdi/testproject/infrastructure/repository/mocks/languages"
	"github.com/khanzadimahdi/testproject/infrastructure/validator"
)

func TestUpdateHandler(t *testing.T) {
	t.Parallel()

	t.Run("update an article", func(t *testing.T) {
		t.Parallel()

		var (
			articleRepository  articles.MockArticlesRepository
			languageRepository languages.MockLanguagesRepository
			requestValidator   validator.MockValidator

			r = updatearticle.Request{
				UUID:         "test-article-uuid",
				Title:        "test title",
				Excerpt:      "test excerpt",
				Body:         "test body",
				AuthorUUID:   "test-author-uuid",
				Tags:         []string{"tag1", "tag2"},
				LanguageCode: "EN",
			}
			existing = article.Article{UUID: r.UUID, LanguageCode: "EN"}
			a        = article.Article{
				UUID:         r.UUID,
				Cover:        r.Cover,
				Video:        r.Video,
				Title:        r.Title,
				Excerpt:      r.Excerpt,
				Body:         r.Body,
				PublishedAt:  r.PublishedAt,
				AuthorUUID:   r.AuthorUUID,
				Tags:         r.Tags,
				LanguageCode: r.LanguageCode,
			}

			u = user.User{
				UUID: r.AuthorUUID,
			}
		)

		requestValidator.On("Validate", &r).Once().Return(nil)
		defer requestValidator.AssertExpectations(t)

		languageRepository.On("Exists", "EN").Once().Return(true)
		defer languageRepository.AssertExpectations(t)

		articleRepository.On("GetOne", r.UUID).Once().Return(existing, nil)
		articleRepository.On("Save", &a).Once().Return(a.UUID, nil)
		defer articleRepository.AssertExpectations(t)

		handler := NewUpdateHandler(updatearticle.NewUseCase(&articleRepository, &languageRepository, &requestValidator))

		var payload bytes.Buffer
		err := json.NewEncoder(&payload).Encode(r)
		assert.NoError(t, err)

		request := httptest.NewRequest(http.MethodPatch, "/", &payload)
		request = request.WithContext(auth.ToContext(request.Context(), &u))
		response := httptest.NewRecorder()

		handler.ServeHTTP(response, request)

		assert.Len(t, response.Body.Bytes(), 0)
		assert.Equal(t, http.StatusNoContent, response.Code)
	})

	t.Run("validation failed", func(t *testing.T) {
		t.Parallel()

		var (
			articleRepository  articles.MockArticlesRepository
			languageRepository languages.MockLanguagesRepository
			requestValidator   validator.MockValidator

			u = user.User{
				UUID: "test-author-uuid",
			}
		)

		requestValidator.On("Validate", &updatearticle.Request{AuthorUUID: u.UUID}).Once().Return(domain.ValidationErrors{
			"body":          "body is required",
			"excerpt":       "excerpt is required",
			"title":         "title is required",
			"language_code": "language is required",
		})
		defer requestValidator.AssertExpectations(t)

		handler := NewUpdateHandler(updatearticle.NewUseCase(&articleRepository, &languageRepository, &requestValidator))

		request := httptest.NewRequest(http.MethodPatch, "/", bytes.NewBufferString("{}"))
		request = request.WithContext(auth.ToContext(request.Context(), &u))
		response := httptest.NewRecorder()

		handler.ServeHTTP(response, request)

		articleRepository.AssertNotCalled(t, "Save")

		expected, err := os.ReadFile("testdata/update-article-validation-errors-response.json")
		assert.NoError(t, err)

		assert.Equal(t, "application/json", response.Header().Get("content-type"))
		assert.JSONEq(t, string(expected), response.Body.String())
		assert.Equal(t, http.StatusBadRequest, response.Code)
	})

	t.Run("error", func(t *testing.T) {
		t.Parallel()

		var (
			articleRepository  articles.MockArticlesRepository
			languageRepository languages.MockLanguagesRepository
			requestValidator   validator.MockValidator

			r = updatearticle.Request{
				UUID:         "test-article-uuid",
				Title:        "test title",
				Excerpt:      "test excerpt",
				Body:         "test body",
				AuthorUUID:   "test-author-uuid",
				Tags:         []string{"tag1", "tag2"},
				LanguageCode: "EN",
			}
			existing = article.Article{UUID: r.UUID, LanguageCode: "EN"}
			a        = article.Article{
				UUID:         r.UUID,
				Cover:        r.Cover,
				Video:        r.Video,
				Title:        r.Title,
				Excerpt:      r.Excerpt,
				Body:         r.Body,
				PublishedAt:  r.PublishedAt,
				AuthorUUID:   r.AuthorUUID,
				Tags:         r.Tags,
				LanguageCode: r.LanguageCode,
			}

			u = user.User{
				UUID: r.AuthorUUID,
			}
		)

		requestValidator.On("Validate", &r).Once().Return(nil)
		defer requestValidator.AssertExpectations(t)

		languageRepository.On("Exists", "EN").Once().Return(true)
		defer languageRepository.AssertExpectations(t)

		articleRepository.On("GetOne", r.UUID).Once().Return(existing, nil)
		articleRepository.On("Save", &a).Once().Return("", errors.New("unexpected error"))
		defer articleRepository.AssertExpectations(t)

		handler := NewUpdateHandler(updatearticle.NewUseCase(&articleRepository, &languageRepository, &requestValidator))

		var payload bytes.Buffer
		err := json.NewEncoder(&payload).Encode(r)
		assert.NoError(t, err)

		request := httptest.NewRequest(http.MethodPatch, "/", &payload)
		request = request.WithContext(auth.ToContext(request.Context(), &u))
		response := httptest.NewRecorder()

		handler.ServeHTTP(response, request)

		assert.Len(t, response.Body.Bytes(), 0)
		assert.Equal(t, http.StatusInternalServerError, response.Code)
	})
}
