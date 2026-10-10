package container

import (
	"net/http"

	"github.com/khanzadimahdi/testproject/application/dashboard/workload/container/getContainers"
	"github.com/khanzadimahdi/testproject/presentation/http/blog/api/dashboard/workload"
)

type indexHandler struct {
	useCase *getContainers.UseCase
	owner   workload.Owner
}

func NewIndexHandler(useCase *getContainers.UseCase, owner workload.Owner) *indexHandler {
	return &indexHandler{useCase: useCase, owner: owner}
}

// @Summary		List containers across Docker VMs
// @Description	the containers of every running Docker VM, each with the VM it is in, narrowed to one VM with ?vm=: anybody's on the workload routes, the caller's own on the my routes
// @Tags			dashboard workload containers
// @Produce		json
// @Param			vm	query		string	false	"Only the containers of this Docker VM"
// @Success		200	{object}	getContainers.Response
// @Failure		400	{object}	workload.Refusal
// @Failure		500	{object}	workload.Failure
// @Router			/dashboard/workload/containers [get]
// @Router			/dashboard/my/workload/containers [get]
func (h *indexHandler) ServeHTTP(rw http.ResponseWriter, r *http.Request) {
	response, err := h.useCase.Execute(r.Context(), &getContainers.Request{
		VMUUID:    r.URL.Query().Get("vm"),
		OwnerUUID: h.owner(r),
	})

	switch {
	case workload.Failed(rw, r, err):
	case workload.Refused(rw, response.ValidationErrors):
	default:
		workload.JSON(rw, http.StatusOK, response)
	}
}
