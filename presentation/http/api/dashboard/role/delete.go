package role

import (
	"errors"
	"net/http"

	deleterole "github.com/khanzadimahdi/testproject/application/dashboard/role/deleteRole"
	"github.com/khanzadimahdi/testproject/domain"
)

type deleteHandler struct {
	useCase *deleterole.UseCase
}

func NewDeleteHandler(useCase *deleterole.UseCase) *deleteHandler {
	return &deleteHandler{
		useCase: useCase,
	}
}

func (h *deleteHandler) ServeHTTP(rw http.ResponseWriter, r *http.Request) {
	UUID := r.PathValue("uuid")

	request := &deleterole.Request{
		RoleUUID: UUID,
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
