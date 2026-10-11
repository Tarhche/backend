package network

import (
	"net/http"

	"github.com/khanzadimahdi/testproject/application/dashboard/workload/network/deleteNetwork"
	"github.com/khanzadimahdi/testproject/presentation/http/blog/api/dashboard/workload"
)

type deleteHandler struct {
	useCase *deleteNetwork.UseCase
	owner   workload.Owner
}

func NewDeleteHandler(useCase *deleteNetwork.UseCase, owner workload.Owner) *deleteHandler {
	return &deleteHandler{useCase: useCase, owner: owner}
}

// @Summary		Remove a network
// @Description	remove a docker network from a Docker VM; one a container is attached to is refused, unless cascade deletes the stacks and the containers that need it first. When anything needs it, it is answered with what does, and with whether one of them makes it again, as a stack or a container that owns it does
// @Tags			dashboard workload networks
// @Param			uuid	path		string	true	"VM UUID"
// @Param			id	path		string	true	"Network id or name"
// @Param			cascade	query		bool	false	"Delete the stacks and the containers that need it first"
// @Success		202		{object}	workload.Removal
// @Success		204		{object}	map[string]interface{}
// @Failure		400		{object}	workload.Removal
// @Failure		404		{object}	workload.Failure
// @Failure		500		{object}	workload.Failure
// @Router			/dashboard/workload/vms/{uuid}/networks/{id} [delete]
// @Router			/dashboard/my/workload/vms/{uuid}/networks/{id} [delete]
func (h *deleteHandler) ServeHTTP(rw http.ResponseWriter, r *http.Request) {
	response, err := h.useCase.Execute(r.Context(), &deleteNetwork.Request{
		VMUUID:    r.PathValue("uuid"),
		ID:        r.PathValue("id"),
		OwnerUUID: h.owner(r),
		Cascade:   workload.Flag(r, "cascade"),
	})

	switch {
	case workload.Failed(rw, r, err):
	case workload.Removed(rw, response.ValidationErrors, response.Dependents, response.Recreated):
	case workload.Refused(rw, response.ValidationErrors):
	default:
		rw.WriteHeader(http.StatusNoContent)
	}
}
