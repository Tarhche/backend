package container

import (
	"net/http"

	"github.com/khanzadimahdi/testproject/application/dashboard/workload/container/stopContainer"
	"github.com/khanzadimahdi/testproject/presentation/http/blog/api/dashboard/workload"
)

type stopHandler struct {
	useCase *stopContainer.UseCase
	owner   workload.Owner
}

func NewStopHandler(useCase *stopContainer.UseCase, owner workload.Owner) *stopHandler {
	return &stopHandler{useCase: useCase, owner: owner}
}

// @Summary		Stop a container
// @Description	stop a container, giving it docker's grace period to shut down on its own first
// @Tags			dashboard workload containers
// @Param			uuid	path		string	true	"VM UUID"
// @Param			id		path		string	true	"Container id or name"
// @Success		204		{object}	map[string]interface{}
// @Failure		400		{object}	workload.Refusal
// @Failure		404		{object}	map[string]interface{}
// @Failure		500		{object}	map[string]interface{}
// @Router			/dashboard/workload/vms/{uuid}/containers/{id}/stop [post]
// @Router			/dashboard/my/workload/vms/{uuid}/containers/{id}/stop [post]
func (h *stopHandler) ServeHTTP(rw http.ResponseWriter, r *http.Request) {
	response, err := h.useCase.Execute(r.Context(), &stopContainer.Request{
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
