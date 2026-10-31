package container

import (
	"net/http"

	"github.com/khanzadimahdi/testproject/application/dashboard/workload/container/getVMContainers"
	"github.com/khanzadimahdi/testproject/presentation/http/blog/api/dashboard/workload"
)

type vmIndexHandler struct {
	useCase *getVMContainers.UseCase
	owner   workload.Owner
}

func NewVMIndexHandler(useCase *getVMContainers.UseCase, owner workload.Owner) *vmIndexHandler {
	return &vmIndexHandler{useCase: useCase, owner: owner}
}

// @Summary		List a Docker VM's containers
// @Description	every container of one Docker VM, stopped ones too, read from its dockerd as it is now
// @Tags			dashboard workload containers
// @Produce		json
// @Param			uuid	path		string	true	"VM UUID"
// @Success		200		{object}	getVMContainers.Response
// @Failure		400		{object}	workload.Refusal
// @Failure		404		{object}	workload.Failure
// @Failure		500		{object}	workload.Failure
// @Router			/dashboard/workload/vms/{uuid}/containers [get]
// @Router			/dashboard/my/workload/vms/{uuid}/containers [get]
func (h *vmIndexHandler) ServeHTTP(rw http.ResponseWriter, r *http.Request) {
	response, err := h.useCase.Execute(r.Context(), &getVMContainers.Request{
		VMUUID:    r.PathValue("uuid"),
		OwnerUUID: h.owner(r),
	})

	switch {
	case workload.Failed(rw, r, err):
	case workload.Refused(rw, response.ValidationErrors):
	default:
		workload.JSON(rw, http.StatusOK, response)
	}
}
