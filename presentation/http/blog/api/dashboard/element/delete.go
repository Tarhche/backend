package element

import (
	"errors"
	"net/http"

	deleteElement "github.com/khanzadimahdi/testproject/application/dashboard/element/deleteElement"
	"github.com/khanzadimahdi/testproject/domain"
)

type deleteHandler struct {
	useCase *deleteElement.UseCase
}

func NewDeleteHandler(useCase *deleteElement.UseCase) *deleteHandler {
	return &deleteHandler{
		useCase: useCase,
	}
}

// @Summary		Delete element
// @Description	remove element identified by UUID
// @Tags			dashboard elements
// @Accept			json
// @Produce		json
// @Param			uuid	path	string	true	"Element UUID"
// @Success		200
// @Failure		404	{object}	map[string]interface{}
// @Failure		500	{object}	map[string]interface{}
// @Router			/dashboard/elements/{uuid} [delete]
func (h *deleteHandler) ServeHTTP(rw http.ResponseWriter, r *http.Request) {
	UUID := r.PathValue("uuid")

	request := &deleteElement.Request{
		ElementUUID: UUID,
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
