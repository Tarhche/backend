package vm

import (
	"net/http"

	"github.com/khanzadimahdi/testproject/application/dashboard/workload/vm/updateVM"
	"github.com/khanzadimahdi/testproject/presentation/http/blog/api/dashboard/workload"
)

type updateHandler struct {
	useCase *updateVM.UseCase
	owner   workload.Owner
}

func NewUpdateHandler(useCase *updateVM.UseCase, owner workload.Owner) *updateHandler {
	return &updateHandler{useCase: useCase, owner: owner}
}

// @Summary		Change a VM
// @Description	change what the request carries and leave the rest: name, lifetime_seconds, ports, network and resources (sent whole). A change to the ports, the network or the resources restarts a VM that is not stopped; the disk only grows
// @Tags			dashboard workload vms
// @Accept			json
// @Produce		json
// @Param			uuid	path		string				true	"VM UUID"
// @Param			body	body		updateVM.Request	true	"What changes"
// @Success		200		{object}	presenter.VM
// @Failure		400		{object}	workload.Refusal
// @Failure		404		{object}	workload.Failure
// @Failure		500		{object}	workload.Failure
// @Router			/dashboard/workload/vms/{uuid} [patch]
// @Router			/dashboard/my/workload/vms/{uuid} [patch]
func (h *updateHandler) ServeHTTP(rw http.ResponseWriter, r *http.Request) {
	var request updateVM.Request
	if !workload.Decode(rw, r, &request) {
		return
	}

	request.UUID = r.PathValue("uuid")
	request.OwnerUUID = h.owner(r)

	response, err := h.useCase.Execute(r.Context(), &request)

	switch {
	case workload.Failed(rw, r, err):
	case workload.Refused(rw, response.ValidationErrors):
	default:
		workload.JSON(rw, http.StatusOK, response.VM)
	}
}
