package container

import (
	"net/http"

	"github.com/khanzadimahdi/testproject/application/dashboard/workload/container/deleteContainer"
	"github.com/khanzadimahdi/testproject/presentation/http/blog/api/dashboard/workload"
)

type deleteHandler struct {
	useCase *deleteContainer.UseCase
	owner   workload.Owner
}

func NewDeleteHandler(useCase *deleteContainer.UseCase, owner workload.Owner) *deleteHandler {
	return &deleteHandler{useCase: useCase, owner: owner}
}

// @Summary		Remove a container
// @Description	remove a container; one that is running only by force. Its volumes stay
// @Tags			dashboard workload containers
// @Param			uuid	path		string	true	"VM UUID"
// @Param			id		path		string	true	"Container id or name"
// @Param			force	query		bool	false	"Remove it even while it runs"
// @Success		204		{object}	map[string]interface{}
// @Failure		400		{object}	workload.Refusal
// @Failure		404		{object}	workload.Failure
// @Failure		500		{object}	workload.Failure
// @Router			/dashboard/workload/vms/{uuid}/containers/{id} [delete]
// @Router			/dashboard/my/workload/vms/{uuid}/containers/{id} [delete]
func (h *deleteHandler) ServeHTTP(rw http.ResponseWriter, r *http.Request) {
	response, err := h.useCase.Execute(r.Context(), &deleteContainer.Request{
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
