package article

import (
	"errors"
	"net/http"

	"github.com/julienschmidt/httprouter"
	deletearticle "github.com/khanzadimahdi/testproject.git/application/article/deleteArticle"
	"github.com/khanzadimahdi/testproject.git/domain"
)

type deleteHandler struct {
	deleteArticleUseCase *deletearticle.UseCase
}

func NewDeleteHandler(deleteArticleUseCase *deletearticle.UseCase) *deleteHandler {
	return &deleteHandler{
		deleteArticleUseCase: deleteArticleUseCase,
	}
}

func (h *deleteHandler) ServeHTTP(rw http.ResponseWriter, r *http.Request) {
	UUID := httprouter.ParamsFromContext(r.Context()).ByName("uuid")

	request := deletearticle.Request{
		ArticleUUID: UUID,
	}

	err := h.deleteArticleUseCase.DeleteArticle(request)
	switch true {
	case errors.Is(err, domain.ErrNotExists):
		rw.WriteHeader(http.StatusNotFound)
	default:
		rw.WriteHeader(http.StatusOK)
	}
}
