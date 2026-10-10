package container

import (
	"net/http"

	"github.com/khanzadimahdi/testproject/application/dashboard/workload/container/disconnectNetwork"
	"github.com/khanzadimahdi/testproject/presentation/http/blog/api/dashboard/workload"
)

type disconnectHandler struct {
	useCase *disconnectNetwork.UseCase
	owner   workload.Owner
}

func NewDisconnectHandler(useCase *disconnectNetwork.UseCase, owner workload.Owner) *disconnectHandler {
	return &disconnectHandler{useCase: useCase, owner: owner}
}

// @Summary		Disconnect a container from a network
// @Description	detach a container from one of its VM's docker networks
// @Tags			dashboard workload containers
// @Param			uuid	path		string	true	"VM UUID"
// @Param			id		path		string	true	"Container id or name"
// @Param			network	path		string	true	"Network id or name"
// @Success		204		{object}	map[string]interface{}
// @Failure		400		{object}	workload.Refusal
// @Failure		404		{object}	workload.Failure
// @Failure		500		{object}	workload.Failure
// @Router			/dashboard/workload/vms/{uuid}/containers/{id}/networks/{network} [delete]
// @Router			/dashboard/my/workload/vms/{uuid}/containers/{id}/networks/{network} [delete]
func (h *disconnectHandler) ServeHTTP(rw http.ResponseWriter, r *http.Request) {
	response, err := h.useCase.Execute(r.Context(), &disconnectNetwork.Request{
		VMUUID:    r.PathValue("uuid"),
		ID:        r.PathValue("id"),
		Network:   r.PathValue("network"),
		OwnerUUID: h.owner(r),
	})

	switch {
	case workload.Failed(rw, r, err):
	case workload.Refused(rw, response.ValidationErrors):
	default:
		rw.WriteHeader(http.StatusNoContent)
	}
}
