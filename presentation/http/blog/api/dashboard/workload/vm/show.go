package vm

import (
	"net/http"

	"github.com/khanzadimahdi/testproject/application/dashboard/workload/vm/getVM"
	"github.com/khanzadimahdi/testproject/presentation/http/blog/api/dashboard/workload"
)

type showHandler struct {
	useCase *getVM.UseCase
	owner   workload.Owner
}

func NewShowHandler(useCase *getVM.UseCase, owner workload.Owner) *showHandler {
	return &showHandler{useCase: useCase, owner: owner}
}

// @Summary		Show a VM
// @Description	one VM, with the addresses its ports are served on while its ingress is allowed
// @Tags			dashboard workload vms
// @Produce		json
// @Param			uuid	path		string	true	"VM UUID"
// @Success		200		{object}	getVM.Response
// @Failure		404		{object}	workload.Failure
// @Failure		500		{object}	workload.Failure
// @Router			/dashboard/workload/vms/{uuid} [get]
// @Router			/dashboard/my/workload/vms/{uuid} [get]
func (h *showHandler) ServeHTTP(rw http.ResponseWriter, r *http.Request) {
	response, err := h.useCase.Execute(r.Context(), &getVM.Request{
		UUID:      r.PathValue("uuid"),
		OwnerUUID: h.owner(r),
	})

	if workload.Failed(rw, r, err) {
		return
	}

	workload.JSON(rw, http.StatusOK, response)
}
