package comment

import (
	"errors"
	"net/http"

	"github.com/khanzadimahdi/testproject/application/auth"
	"github.com/khanzadimahdi/testproject/application/dashboard/comment/deleteUserComment"
	"github.com/khanzadimahdi/testproject/domain"
	"github.com/khanzadimahdi/testproject/domain/permission"
)

type deleteUserHandler struct {
	useCase    *deleteUserComment.UseCase
	authorizer domain.Authorizer
}

func NewDeleteUserCommentHandler(useCase *deleteUserComment.UseCase, a domain.Authorizer) *deleteUserHandler {
	return &deleteUserHandler{
		useCase:    useCase,
		authorizer: a,
	}
}

func (h *deleteUserHandler) ServeHTTP(rw http.ResponseWriter, r *http.Request) {
	userUUID := auth.FromContext(r.Context()).UUID
	if ok, err := h.authorizer.Authorize(userUUID, permission.SelfCommentsDelete); err != nil {
		rw.WriteHeader(http.StatusInternalServerError)
		return
	} else if !ok {
		rw.WriteHeader(http.StatusUnauthorized)
		return
	}

	UUID := r.PathValue("uuid")

	request := deleteUserComment.Request{
		CommentUUID: UUID,
		UserUUID:    userUUID,
	}

	err := h.useCase.Execute(request)
	switch {
	case errors.Is(err, domain.ErrNotExists):
		rw.WriteHeader(http.StatusNotFound)
	case err != nil:
		rw.WriteHeader(http.StatusInternalServerError)
	default:
		rw.WriteHeader(http.StatusOK)
	}
}
