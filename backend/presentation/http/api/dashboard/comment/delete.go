package comment

import (
	"errors"
	"net/http"

	"github.com/julienschmidt/httprouter"

	"github.com/khanzadimahdi/testproject/application/auth"
	"github.com/khanzadimahdi/testproject/application/dashboard/comment/deleteComment"
	"github.com/khanzadimahdi/testproject/domain"
	"github.com/khanzadimahdi/testproject/domain/permission"
)

type deleteHandler struct {
	deleteCommentUseCase *deleteComment.UseCase
	authorizer           domain.Authorizer
}

func NewDeleteHandler(deleteCommentUseCase *deleteComment.UseCase, a domain.Authorizer) *deleteHandler {
	return &deleteHandler{
		deleteCommentUseCase: deleteCommentUseCase,
		authorizer:           a,
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

	UUID := httprouter.ParamsFromContext(r.Context()).ByName("uuid")

	request := deleteComment.Request{
		CommentUUID: UUID,
	}

	err := h.deleteCommentUseCase.Execute(request)
	switch true {
	case errors.Is(err, domain.ErrNotExists):
		rw.WriteHeader(http.StatusNotFound)
	case err != nil:
		rw.WriteHeader(http.StatusInternalServerError)
	default:
		rw.WriteHeader(http.StatusOK)
	}
}
