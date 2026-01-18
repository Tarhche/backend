package file

import (
	"errors"
	"net/http"

	"github.com/khanzadimahdi/testproject/application/auth"
	deleteuserfile "github.com/khanzadimahdi/testproject/application/dashboard/file/deleteUserFile"
	"github.com/khanzadimahdi/testproject/domain"
)

type deleteUserHandler struct {
	useCase *deleteuserfile.UseCase
}

func NewDeleteUserHandler(useCase *deleteuserfile.UseCase) *deleteUserHandler {
	return &deleteUserHandler{
		useCase: useCase,
	}
}

func (h *deleteUserHandler) ServeHTTP(rw http.ResponseWriter, r *http.Request) {
	userUUID := auth.FromContext(r.Context()).UUID

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
