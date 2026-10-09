package vm

import (
	"net/http"

	"github.com/khanzadimahdi/testproject/application/dashboard/workload/vm/getVMs"
	"github.com/khanzadimahdi/testproject/presentation/http/blog/api/dashboard/workload"
)

type indexHandler struct {
	useCase *getVMs.UseCase
	owner   workload.Owner
}

func NewIndexHandler(useCase *getVMs.UseCase, owner workload.Owner) *indexHandler {
	return &indexHandler{useCase: useCase, owner: owner}
}

// @Summary		List VMs
// @Description	a page of VMs: anybody's on the workload routes, the caller's own on the my routes
// @Tags			dashboard workload vms
// @Produce		json
// @Param			page	query		int		false	"Page"	default(1)
// @Success		200		{object}	getVMs.Response
// @Failure		400		{object}	workload.Refusal
// @Failure		500		{object}	workload.Failure
// @Router			/dashboard/workload/vms [get]
// @Router			/dashboard/my/workload/vms [get]
func (h *indexHandler) ServeHTTP(rw http.ResponseWriter, r *http.Request) {
	response, err := h.useCase.Execute(r.Context(), &getVMs.Request{
		Page:      workload.Page(r),
		OwnerUUID: h.owner(r),
	})

	switch {
	case workload.Failed(rw, r, err):
	case workload.Refused(rw, response.ValidationErrors):
	default:
		workload.JSON(rw, http.StatusOK, response)
	}
}
