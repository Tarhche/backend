package user

import (
	"errors"
	"net/http"

	"github.com/khanzadimahdi/testproject/application/auth"
	deleteuser "github.com/khanzadimahdi/testproject/application/dashboard/user/deleteUser"
	"github.com/khanzadimahdi/testproject/domain"
	"github.com/khanzadimahdi/testproject/domain/permission"
)

type deleteHandler struct {
	useCase    *deleteuser.UseCase
	authorizer domain.Authorizer
}

func NewDeleteHandler(useCase *deleteuser.UseCase, a domain.Authorizer) *deleteHandler {
	return &deleteHandler{
		useCase:    useCase,
		authorizer: a,
	}
}

func (h *deleteHandler) ServeHTTP(rw http.ResponseWriter, r *http.Request) {
	currentUserUUID := auth.FromContext(r.Context()).UUID
	if ok, err := h.authorizer.Authorize(currentUserUUID, permission.UsersDelete); err != nil {
		rw.WriteHeader(http.StatusInternalServerError)
		return
	} else if !ok {
		rw.WriteHeader(http.StatusUnauthorized)
		return
	}

	UUID := r.PathValue("uuid")

	request := &deleteuser.Request{
		UserUUID: UUID,
	}

	err := h.useCase.Execute(request)
	switch {
	case errors.Is(err, domain.ErrNotExists):
		rw.WriteHeader(http.StatusNotFound)
	case err != nil:
		rw.WriteHeader(http.StatusInternalServerError)
	default:
		rw.WriteHeader(http.StatusNoContent)
	}
}
