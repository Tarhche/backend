package vm

import (
	"net/http"

	"github.com/khanzadimahdi/testproject/application/dashboard/workload/vm/restartVM"
	"github.com/khanzadimahdi/testproject/presentation/http/blog/api/dashboard/workload"
)

type restartHandler struct {
	useCase *restartVM.UseCase
	owner   workload.Owner
}

func NewRestartHandler(useCase *restartVM.UseCase, owner workload.Owner) *restartHandler {
	return &restartHandler{useCase: useCase, owner: owner}
}

// @Summary		Restart a VM
// @Description	ask for a VM to be stopped and started again in place
// @Tags			dashboard workload vms
// @Param			uuid	path		string	true	"VM UUID"
// @Success		202		{object}	map[string]interface{}
// @Failure		400		{object}	workload.Refusal
// @Failure		404		{object}	map[string]interface{}
// @Failure		500		{object}	map[string]interface{}
// @Router			/dashboard/workload/vms/{uuid}/restart [post]
// @Router			/dashboard/my/workload/vms/{uuid}/restart [post]
func (h *restartHandler) ServeHTTP(rw http.ResponseWriter, r *http.Request) {
	response, err := h.useCase.Execute(r.Context(), &restartVM.Request{
		UUID:      r.PathValue("uuid"),
		OwnerUUID: h.owner(r),
	})

	switch {
	case workload.Failed(rw, r, err):
	case workload.Refused(rw, response.ValidationErrors):
	default:
		rw.WriteHeader(http.StatusAccepted)
	}
}
