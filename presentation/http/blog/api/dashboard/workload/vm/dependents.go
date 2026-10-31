package vm

import (
	"net/http"

	"github.com/khanzadimahdi/testproject/application/dashboard/workload/vm/getVMDependents"
	"github.com/khanzadimahdi/testproject/presentation/http/blog/api/dashboard/workload"
)

type dependentsHandler struct {
	useCase *getVMDependents.UseCase
	owner   workload.Owner
}

func NewDependentsHandler(useCase *getVMDependents.UseCase, owner workload.Owner) *dependentsHandler {
	return &dependentsHandler{useCase: useCase, owner: owner}
}

// @Summary		What needs a VM
// @Description	what needs a VM, read before it is asked to be deleted: the containers and the stacks in it, which make it again, under its uuid and as it was, once it is gone, and whether one of them does, as its delete would be answered with them now; a list of none when nothing does. Reading it deletes nothing, and asks nothing of its node
// @Tags			dashboard workload vms
// @Produce		json
// @Param			uuid	path		string	true	"VM UUID"
// @Success		200		{object}	presenter.Needs
// @Failure		400		{object}	workload.Refusal
// @Failure		404		{object}	workload.Failure
// @Failure		500		{object}	workload.Failure
// @Router			/dashboard/workload/vms/{uuid}/dependents [get]
// @Router			/dashboard/my/workload/vms/{uuid}/dependents [get]
func (h *dependentsHandler) ServeHTTP(rw http.ResponseWriter, r *http.Request) {
	response, err := h.useCase.Execute(r.Context(), &getVMDependents.Request{
		UUID:      r.PathValue("uuid"),
		OwnerUUID: h.owner(r),
	})

	switch {
	case workload.Failed(rw, r, err):
	case workload.Refused(rw, response.ValidationErrors):
	default:
		workload.JSON(rw, http.StatusOK, response.Needs)
	}
}
