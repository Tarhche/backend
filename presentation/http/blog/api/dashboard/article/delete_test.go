package article

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"

	deletearticle "github.com/khanzadimahdi/testproject/application/dashboard/article/deleteArticle"
	"github.com/khanzadimahdi/testproject/infrastructure/repository/mocks/articles"
)

func TestDeleteHandler(t *testing.T) {
	t.Parallel()

	t.Run("delete an article", func(t *testing.T) {
		t.Parallel()

		var (
			articleRepository articles.MockArticlesRepository

			r = deletearticle.Request{CorrelationUUID: "correlation-uuid", LanguageCode: "EN"}
		)

		articleRepository.On("DeleteByCorrelationUUIDAndLanguage", mock.Anything, r.CorrelationUUID, r.LanguageCode).Return(nil)
		defer articleRepository.AssertExpectations(t)

		handler := NewDeleteHandler(deletearticle.NewUseCase(&articleRepository))

		request := httptest.NewRequest(http.MethodPost, "/", nil)
		request.SetPathValue("correlationUUID", r.CorrelationUUID)
		request.SetPathValue("language_code", r.LanguageCode)
		response := httptest.NewRecorder()

		handler.ServeHTTP(response, request)

		assert.Len(t, response.Body.Bytes(), 0)
		assert.Equal(t, http.StatusNoContent, response.Code)
	})
}
