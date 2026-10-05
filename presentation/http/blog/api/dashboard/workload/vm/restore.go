package vm

import (
	"net/http"

	"github.com/khanzadimahdi/testproject/application/dashboard/workload/vm/restoreVM"
	"github.com/khanzadimahdi/testproject/presentation/http/blog/api/dashboard/workload"
)

type restoreHandler struct {
	useCase *restoreVM.UseCase
	owner   workload.Owner
}

func NewRestoreHandler(useCase *restoreVM.UseCase, owner workload.Owner) *restoreHandler {
	return &restoreHandler{useCase: useCase, owner: owner}
}

// @Summary		Restore a VM from a snapshot
// @Description	replace a VM's disk with a snapshot's: one of its owner's, of the same kind and engine, and no larger than its disk. The VM is stopped, restored and started again; it keeps its uuid, its slug and its ports
// @Tags			dashboard workload vms
// @Accept			json
// @Param			uuid	path		string				true	"VM UUID"
// @Param			body	body		restoreVM.Request	true	"The snapshot"
// @Success		202		{object}	map[string]interface{}
// @Failure		400		{object}	workload.Refusal
// @Failure		404		{object}	workload.Failure
// @Failure		500		{object}	workload.Failure
// @Router			/dashboard/workload/vms/{uuid}/restore [post]
// @Router			/dashboard/my/workload/vms/{uuid}/restore [post]
func (h *restoreHandler) ServeHTTP(rw http.ResponseWriter, r *http.Request) {
	var request restoreVM.Request
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
		rw.WriteHeader(http.StatusAccepted)
	}
}
