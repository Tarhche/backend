package file

import (
	"errors"
	"net/http"

	"github.com/khanzadimahdi/testproject/application/auth"
	deleteuserfile "github.com/khanzadimahdi/testproject/application/dashboard/file/deleteUserFile"
	"github.com/khanzadimahdi/testproject/domain"
	"github.com/khanzadimahdi/testproject/domain/permission"
)

type deleteUserHandler struct {
	useCase    *deleteuserfile.UseCase
	authorizer domain.Authorizer
}

func NewDeleteUserHandler(useCase *deleteuserfile.UseCase, a domain.Authorizer) *deleteUserHandler {
	return &deleteUserHandler{
		useCase:    useCase,
		authorizer: a,
	}
}

func (h *deleteUserHandler) ServeHTTP(rw http.ResponseWriter, r *http.Request) {
	userUUID := auth.FromContext(r.Context()).UUID
	if ok, err := h.authorizer.Authorize(userUUID, permission.SelfFilesDelete); err != nil {
		rw.WriteHeader(http.StatusInternalServerError)
		return
	} else if !ok {
		rw.WriteHeader(http.StatusForbidden)
		return
	}

	UUID := r.PathValue("uuid")

	err := h.useCase.Execute(deleteuserfile.Request{
		OwnerUUID: userUUID,
		FileUUID:  UUID,
	})

	switch {
	case errors.Is(err, domain.ErrNotExists):
		rw.WriteHeader(http.StatusNotFound)
	case err != nil:
		rw.WriteHeader(http.StatusInternalServerError)
	default:
		rw.WriteHeader(http.StatusNoContent)
	}
}
