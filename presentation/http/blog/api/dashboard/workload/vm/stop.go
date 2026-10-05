package vm

import (
	"net/http"

	"github.com/khanzadimahdi/testproject/application/dashboard/workload/vm/stopVM"
	"github.com/khanzadimahdi/testproject/presentation/http/blog/api/dashboard/workload"
)

type stopHandler struct {
	useCase *stopVM.UseCase
	owner   workload.Owner
}

func NewStopHandler(useCase *stopVM.UseCase, owner workload.Owner) *stopHandler {
	return &stopHandler{useCase: useCase, owner: owner}
}

// @Summary		Stop a VM
// @Description	ask for a VM to be stopped; its disk is kept where it lives
// @Tags			dashboard workload vms
// @Param			uuid	path		string	true	"VM UUID"
// @Success		202		{object}	map[string]interface{}
// @Failure		400		{object}	workload.Refusal
// @Failure		404		{object}	map[string]interface{}
// @Failure		500		{object}	map[string]interface{}
// @Router			/dashboard/workload/vms/{uuid}/stop [post]
// @Router			/dashboard/my/workload/vms/{uuid}/stop [post]
func (h *stopHandler) ServeHTTP(rw http.ResponseWriter, r *http.Request) {
	response, err := h.useCase.Execute(r.Context(), &stopVM.Request{
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
