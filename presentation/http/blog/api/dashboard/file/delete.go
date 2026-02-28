package file

import (
	"errors"
	"net/http"

	deletefile "github.com/khanzadimahdi/testproject/application/dashboard/file/deleteFile"
	"github.com/khanzadimahdi/testproject/domain"
)

type deleteHandler struct {
	useCase *deletefile.UseCase
}

func NewDeleteHandler(useCase *deletefile.UseCase) *deleteHandler {
	return &deleteHandler{
		useCase: useCase,
	}
}

// @Summary		Delete file
// @Description	remove file by UUID
// @Tags			dashboard files
// @Accept			json
// @Produce		json
// @Param			uuid	path	string	true	"File UUID"
// @Success		204
// @Failure		404	{object}	map[string]interface{}
// @Failure		500	{object}	map[string]interface{}
// @Router			/dashboard/files/{uuid} [delete]
func (h *deleteHandler) ServeHTTP(rw http.ResponseWriter, r *http.Request) {
	UUID := r.PathValue("uuid")

	err := h.useCase.Execute(deletefile.Request{
		FileUUID: UUID,
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
