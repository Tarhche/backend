package image

import (
	"net/http"

	"github.com/khanzadimahdi/testproject/application/dashboard/workload/image/deleteImage"
	"github.com/khanzadimahdi/testproject/presentation/http/blog/api/dashboard/workload"
)

type deleteHandler struct {
	useCase *deleteImage.UseCase
	owner   workload.Owner
}

func NewDeleteHandler(useCase *deleteImage.UseCase, owner workload.Owner) *deleteHandler {
	return &deleteHandler{useCase: useCase, owner: owner}
}

// @Summary		Remove an image
// @Description	remove an image from a Docker VM; one a container was created from only by force
// @Tags			dashboard workload images
// @Param			uuid	path		string	true	"VM UUID"
// @Param			id	path		string	true	"Image id or tag"
// @Param			force	query		bool	false	"Remove it even while a container uses it"
// @Success		204		{object}	map[string]interface{}
// @Failure		400		{object}	workload.Refusal
// @Failure		404		{object}	workload.Failure
// @Failure		500		{object}	workload.Failure
// @Router			/dashboard/workload/vms/{uuid}/images/{id} [delete]
// @Router			/dashboard/my/workload/vms/{uuid}/images/{id} [delete]
func (h *deleteHandler) ServeHTTP(rw http.ResponseWriter, r *http.Request) {
	response, err := h.useCase.Execute(r.Context(), &deleteImage.Request{
		VMUUID:    r.PathValue("uuid"),
		ID:        r.PathValue("id"),
		Force:     workload.Flag(r, "force"),
		OwnerUUID: h.owner(r),
	})

	switch {
	case workload.Failed(rw, r, err):
	case workload.Refused(rw, response.ValidationErrors):
	default:
		rw.WriteHeader(http.StatusNoContent)
	}
}
