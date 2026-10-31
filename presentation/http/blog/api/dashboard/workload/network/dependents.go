package network

import (
	"net/http"

	"github.com/khanzadimahdi/testproject/application/dashboard/workload/network/getNetworkDependents"
	"github.com/khanzadimahdi/testproject/presentation/http/blog/api/dashboard/workload"
)

type dependentsHandler struct {
	useCase *getNetworkDependents.UseCase
	owner   workload.Owner
}

func NewDependentsHandler(useCase *getNetworkDependents.UseCase, owner workload.Owner) *dependentsHandler {
	return &dependentsHandler{useCase: useCase, owner: owner}
}

// @Summary		What needs a network
// @Description	what needs a docker network of a Docker VM, found by its id or its name as its delete finds it, read before it is asked to be deleted: the containers and the stacks that need it, each saying whether it makes it again once it is gone, as one that owns it does, and whether one of them does, as its delete would be answered with them now; a list of none when nothing does. Reading it deletes nothing, and asks nothing of its node
// @Tags			dashboard workload networks
// @Produce		json
// @Param			uuid	path		string	true	"VM UUID"
// @Param			id	path		string	true	"Network id or name"
// @Success		200		{object}	presenter.Needs
// @Failure		400		{object}	workload.Refusal
// @Failure		404		{object}	workload.Failure
// @Failure		500		{object}	workload.Failure
// @Router			/dashboard/workload/vms/{uuid}/networks/{id}/dependents [get]
// @Router			/dashboard/my/workload/vms/{uuid}/networks/{id}/dependents [get]
func (h *dependentsHandler) ServeHTTP(rw http.ResponseWriter, r *http.Request) {
	response, err := h.useCase.Execute(r.Context(), &getNetworkDependents.Request{
		VMUUID:    r.PathValue("uuid"),
		ID:        r.PathValue("id"),
		OwnerUUID: h.owner(r),
	})

	switch {
	case workload.Failed(rw, r, err):
	case workload.Refused(rw, response.ValidationErrors):
	default:
		workload.JSON(rw, http.StatusOK, response.Needs)
	}
}
