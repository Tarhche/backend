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

// @Summary		Delete role
// @Description	remove a role by UUID
// @Tags			dashboard roles
// @Accept			json
// @Produce		json
// @Param			uuid	path	string	true	"Role UUID"
// @Success		204
// @Failure		404	{object}	map[string]interface{}
// @Failure		500	{object}	map[string]interface{}
// @Router			/dashboard/roles/{uuid} [delete]
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
