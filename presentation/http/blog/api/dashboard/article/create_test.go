package article

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/khanzadimahdi/testproject/application/auth"
	createarticle "github.com/khanzadimahdi/testproject/application/dashboard/article/createArticle"
	"github.com/khanzadimahdi/testproject/domain"
	"github.com/khanzadimahdi/testproject/domain/article"
	"github.com/khanzadimahdi/testproject/domain/author"
	"github.com/khanzadimahdi/testproject/domain/user"
	"github.com/khanzadimahdi/testproject/infrastructure/repository/mocks/articles"
	"github.com/khanzadimahdi/testproject/infrastructure/validator"
)

func TestCreateHandler(t *testing.T) {
	t.Parallel()

	t.Run("create an article", func(t *testing.T) {
		t.Parallel()

		var (
			articleRepository articles.MockArticlesRepository
			requestValidator  validator.MockValidator

			r = createarticle.Request{
				Title:      "test title",
				Excerpt:    "test excerpt",
				Body:       "test body",
				AuthorUUID: "test-author-uuid",
				Tags:       []string{"tag1", "tag2"},
			}

			u = user.User{
				UUID: r.AuthorUUID,
			}

			a = article.Article{
				Cover:       r.Cover,
				Video:       r.Video,
				Title:       r.Title,
				Excerpt:     r.Excerpt,
				Body:        r.Body,
				PublishedAt: r.PublishedAt,
				Author: author.Author{
					UUID: r.AuthorUUID,
				},
				Tags: r.Tags,
			}

			au = "article-uuid"
		)

		requestValidator.On("Validate", &r).Once().Return(nil)
		defer requestValidator.AssertExpectations(t)

		articleRepository.On("Save", &a).Once().Return(au, nil)
		defer articleRepository.AssertExpectations(t)

		handler := NewCreateHandler(createarticle.NewUseCase(&articleRepository, &requestValidator))

		var payload bytes.Buffer
		err := json.NewEncoder(&payload).Encode(r)
		assert.NoError(t, err)

		request := httptest.NewRequest(http.MethodGet, "/", &payload)
		request = request.WithContext(auth.ToContext(request.Context(), &u))
		response := httptest.NewRecorder()

		handler.ServeHTTP(response, request)

		expectedBody, err := os.ReadFile("testdata/create-article-response.json")
		assert.NoError(t, err)

		assert.Equal(t, "application/json", response.Header().Get("content-type"))
		assert.JSONEq(t, string(expectedBody), response.Body.String())
		assert.Equal(t, http.StatusCreated, response.Code)
	})

	t.Run("validation failed", func(t *testing.T) {
		t.Parallel()

		var (
			articleRepository articles.MockArticlesRepository
			requestValidator  validator.MockValidator

			u = user.User{
				UUID: "test-author-uuid",
			}
		)

		requestValidator.On("Validate", &createarticle.Request{AuthorUUID: u.UUID}).Once().Return(domain.ValidationErrors{
			"body":    "body is required",
			"excerpt": "excerpt is required",
			"title":   "title is required",
		})
		defer requestValidator.AssertExpectations(t)

		handler := NewCreateHandler(createarticle.NewUseCase(&articleRepository, &requestValidator))

		request := httptest.NewRequest(http.MethodGet, "/", bytes.NewBufferString("{}"))
		request = request.WithContext(auth.ToContext(request.Context(), &u))
		response := httptest.NewRecorder()

		handler.ServeHTTP(response, request)

		articleRepository.AssertNotCalled(t, "Save")

		expected, err := os.ReadFile("testdata/create-article-validation-errors-response.json")
		assert.NoError(t, err)

		assert.Equal(t, "application/json", response.Header().Get("content-type"))
		assert.JSONEq(t, string(expected), response.Body.String())
		assert.Equal(t, http.StatusBadRequest, response.Code)
	})
}
