package vm

import (
	"net/http"

	"github.com/khanzadimahdi/testproject/application/dashboard/workload/vm/deleteVM"
	"github.com/khanzadimahdi/testproject/presentation/http/blog/api/dashboard/workload"
)

type deleteHandler struct {
	useCase *deleteVM.UseCase
	owner   workload.Owner
}

func NewDeleteHandler(useCase *deleteVM.UseCase, owner workload.Owner) *deleteHandler {
	return &deleteHandler{useCase: useCase, owner: owner}
}

// @Summary		Delete a VM
// @Description	ask for a VM and its disk to be removed; it is deleting until its node has removed it, and its snapshots stay. The containers in it make it again, under its uuid and as it was, and then themselves in it, unless cascade deletes them with it; when any container needs it, it is answered with them, and with whether one of them makes it again
// @Tags			dashboard workload vms
// @Param			uuid	path		string	true	"VM UUID"
// @Param			cascade	query		bool	false	"Delete the containers in it with it, rather than have them make it again"
// @Success		202		{object}	workload.Removal
// @Failure		400		{object}	workload.Refusal
// @Failure		404		{object}	workload.Failure
// @Failure		500		{object}	workload.Failure
// @Router			/dashboard/workload/vms/{uuid} [delete]
// @Router			/dashboard/my/workload/vms/{uuid} [delete]
func (h *deleteHandler) ServeHTTP(rw http.ResponseWriter, r *http.Request) {
	response, err := h.useCase.Execute(r.Context(), &deleteVM.Request{
		UUID:      r.PathValue("uuid"),
		OwnerUUID: h.owner(r),
		Cascade:   workload.Flag(r, "cascade"),
	})

	switch {
	case workload.Failed(rw, r, err):
	case workload.Removed(rw, response.ValidationErrors, response.Dependents, response.Recreated):
	case workload.Refused(rw, response.ValidationErrors):
	default:
		rw.WriteHeader(http.StatusAccepted)
	}
}
