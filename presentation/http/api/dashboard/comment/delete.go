package comment

import (
	"net/http"

	"github.com/khanzadimahdi/testproject/application/dashboard/comment/deleteComment"
)

type deleteHandler struct {
	useCase *deleteComment.UseCase
}

func NewDeleteHandler(useCase *deleteComment.UseCase) *deleteHandler {
	return &deleteHandler{
		useCase: useCase,
	}
}

func (h *deleteHandler) ServeHTTP(rw http.ResponseWriter, r *http.Request) {
	UUID := r.PathValue("uuid")

	request := deleteComment.Request{
		CommentUUID: UUID,
	}

	err := h.useCase.Execute(&request)
	switch {
	case err != nil:
		rw.WriteHeader(http.StatusInternalServerError)
	default:
		rw.WriteHeader(http.StatusNoContent)
	}
}
