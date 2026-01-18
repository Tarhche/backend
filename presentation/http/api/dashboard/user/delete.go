package user

import (
	"errors"
	"net/http"

	deleteuser "github.com/khanzadimahdi/testproject/application/dashboard/user/deleteUser"
	"github.com/khanzadimahdi/testproject/domain"
)

type deleteHandler struct {
	useCase *deleteuser.UseCase
}

func NewDeleteHandler(useCase *deleteuser.UseCase) *deleteHandler {
	return &deleteHandler{
		useCase: useCase,
	}
}

func (h *deleteHandler) ServeHTTP(rw http.ResponseWriter, r *http.Request) {
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
