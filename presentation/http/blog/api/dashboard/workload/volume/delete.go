package volume

import (
	"net/http"

	"github.com/khanzadimahdi/testproject/application/dashboard/workload/volume/deleteVolume"
	"github.com/khanzadimahdi/testproject/presentation/http/blog/api/dashboard/workload"
)

type deleteHandler struct {
	useCase *deleteVolume.UseCase
	owner   workload.Owner
}

func NewDeleteHandler(useCase *deleteVolume.UseCase, owner workload.Owner) *deleteHandler {
	return &deleteHandler{useCase: useCase, owner: owner}
}

// @Summary		Remove a volume
// @Description	remove a volume, and what was kept in it, from a Docker VM; one a container mounts only by force
// @Tags			dashboard workload volumes
// @Param			uuid	path		string	true	"VM UUID"
// @Param			name	path		string	true	"Volume name"
// @Param			force	query		bool	false	"Remove it even while a container uses it"
// @Success		204		{object}	map[string]interface{}
// @Failure		400		{object}	workload.Refusal
// @Failure		404		{object}	map[string]interface{}
// @Failure		500		{object}	map[string]interface{}
// @Router			/dashboard/workload/vms/{uuid}/volumes/{name} [delete]
// @Router			/dashboard/my/workload/vms/{uuid}/volumes/{name} [delete]
func (h *deleteHandler) ServeHTTP(rw http.ResponseWriter, r *http.Request) {
	response, err := h.useCase.Execute(r.Context(), &deleteVolume.Request{
		VMUUID:    r.PathValue("uuid"),
		Name:      r.PathValue("name"),
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
