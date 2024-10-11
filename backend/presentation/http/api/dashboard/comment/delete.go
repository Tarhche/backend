package comment

import (
	"net/http"

	"github.com/khanzadimahdi/testproject/application/auth"
	"github.com/khanzadimahdi/testproject/application/dashboard/comment/deleteComment"
	"github.com/khanzadimahdi/testproject/domain"
	"github.com/khanzadimahdi/testproject/domain/permission"
)

type deleteHandler struct {
	useCase    *deleteComment.UseCase
	authorizer domain.Authorizer
}

func NewDeleteHandler(useCase *deleteComment.UseCase, a domain.Authorizer) *deleteHandler {
	return &deleteHandler{
		useCase:    useCase,
		authorizer: a,
	}
}

func (h *deleteHandler) ServeHTTP(rw http.ResponseWriter, r *http.Request) {
	userUUID := auth.FromContext(r.Context()).UUID
	if ok, err := h.authorizer.Authorize(userUUID, permission.CommentsDelete); err != nil {
		rw.WriteHeader(http.StatusInternalServerError)
		return
	} else if !ok {
		rw.WriteHeader(http.StatusUnauthorized)
		return
	}

	UUID := r.PathValue("uuid")

	request := deleteComment.Request{
		CommentUUID: UUID,
	}

	err := h.useCase.Execute(request)
	switch {
	case err != nil:
		rw.WriteHeader(http.StatusInternalServerError)
	default:
		rw.WriteHeader(http.StatusNoContent)
	}
}
