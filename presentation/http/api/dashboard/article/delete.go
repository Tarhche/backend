package article

import (
	"net/http"

	deletearticle "github.com/khanzadimahdi/testproject/application/dashboard/article/deleteArticle"
)

type deleteHandler struct {
	useCase *deletearticle.UseCase
}

func NewDeleteHandler(useCase *deletearticle.UseCase) *deleteHandler {
	return &deleteHandler{
		useCase: useCase,
	}
}

func (h *deleteHandler) ServeHTTP(rw http.ResponseWriter, r *http.Request) {
	UUID := r.PathValue("uuid")
	request := &deletearticle.Request{
		ArticleUUID: UUID,
	}

	err := h.useCase.Execute(request)
	switch {
	case err != nil:
		rw.WriteHeader(http.StatusInternalServerError)
	default:
		rw.WriteHeader(http.StatusNoContent)
	}
}
