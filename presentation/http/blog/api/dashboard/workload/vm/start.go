package vm

import (
	"net/http"

	"github.com/khanzadimahdi/testproject/application/dashboard/workload/vm/startVM"
	"github.com/khanzadimahdi/testproject/presentation/http/blog/api/dashboard/workload"
)

type startHandler struct {
	useCase *startVM.UseCase
	owner   workload.Owner
}

func NewStartHandler(useCase *startVM.UseCase, owner workload.Owner) *startHandler {
	return &startHandler{useCase: useCase, owner: owner}
}

// @Summary		Start a VM
// @Description	ask for a stopped VM to be started; its state says how it is going
// @Tags			dashboard workload vms
// @Param			uuid	path		string	true	"VM UUID"
// @Success		202		{object}	map[string]interface{}
// @Failure		400		{object}	workload.Refusal
// @Failure		404		{object}	workload.Failure
// @Failure		500		{object}	workload.Failure
// @Router			/dashboard/workload/vms/{uuid}/start [post]
// @Router			/dashboard/my/workload/vms/{uuid}/start [post]
func (h *startHandler) ServeHTTP(rw http.ResponseWriter, r *http.Request) {
	response, err := h.useCase.Execute(r.Context(), &startVM.Request{
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
