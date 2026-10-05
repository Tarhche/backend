package container

import (
	"net/http"

	"github.com/khanzadimahdi/testproject/application/dashboard/workload/container/restartContainer"
	"github.com/khanzadimahdi/testproject/presentation/http/blog/api/dashboard/workload"
)

type restartHandler struct {
	useCase *restartContainer.UseCase
	owner   workload.Owner
}

func NewRestartHandler(useCase *restartContainer.UseCase, owner workload.Owner) *restartHandler {
	return &restartHandler{useCase: useCase, owner: owner}
}

// @Summary		Restart a container
// @Description	stop a container and start it again
// @Tags			dashboard workload containers
// @Param			uuid	path		string	true	"VM UUID"
// @Param			id		path		string	true	"Container id or name"
// @Success		204		{object}	map[string]interface{}
// @Failure		400		{object}	workload.Refusal
// @Failure		404		{object}	map[string]interface{}
// @Failure		500		{object}	map[string]interface{}
// @Router			/dashboard/workload/vms/{uuid}/containers/{id}/restart [post]
// @Router			/dashboard/my/workload/vms/{uuid}/containers/{id}/restart [post]
func (h *restartHandler) ServeHTTP(rw http.ResponseWriter, r *http.Request) {
	response, err := h.useCase.Execute(r.Context(), &restartContainer.Request{
		VMUUID:    r.PathValue("uuid"),
		ID:        r.PathValue("id"),
		OwnerUUID: h.owner(r),
	})

	switch {
	case workload.Failed(rw, r, err):
	case workload.Refused(rw, response.ValidationErrors):
	default:
		rw.WriteHeader(http.StatusNoContent)
	}
}
