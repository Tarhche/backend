package article

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"

	deletearticle "github.com/khanzadimahdi/testproject/application/dashboard/article/deleteArticle"
	"github.com/khanzadimahdi/testproject/infrastructure/repository/mocks/articles"
)

func TestDeleteHandler(t *testing.T) {
	t.Parallel()

	t.Run("delete an article", func(t *testing.T) {
		t.Parallel()

		var (
			articleRepository articles.MockArticlesRepository

			r = deletearticle.Request{ArticleUUID: "article-uuid"}
		)

		articleRepository.On("Delete", r.ArticleUUID).Return(nil)
		defer articleRepository.AssertExpectations(t)

		handler := NewDeleteHandler(deletearticle.NewUseCase(&articleRepository))

		request := httptest.NewRequest(http.MethodPost, "/", nil)
		request.SetPathValue("uuid", r.ArticleUUID)
		response := httptest.NewRecorder()

		handler.ServeHTTP(response, request)

		assert.Len(t, response.Body.Bytes(), 0)
		assert.Equal(t, http.StatusNoContent, response.Code)
	})
}
