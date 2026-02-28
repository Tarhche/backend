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

// @Summary		Delete user
// @Description	remove a user by UUID
// @Tags			dashboard users
// @Accept			json
// @Produce		json
// @Param			uuid	path	string	true	"User UUID"
// @Success		204
// @Failure		404	{object}	map[string]interface{}
// @Failure		500	{object}	map[string]interface{}
// @Router			/dashboard/users/{uuid} [delete]
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
