package container

import (
	"net/http"

	"github.com/khanzadimahdi/testproject/application/dashboard/workload/container/connectNetwork"
	"github.com/khanzadimahdi/testproject/presentation/http/blog/api/dashboard/workload"
)

type connectHandler struct {
	useCase *connectNetwork.UseCase
	owner   workload.Owner
}

func NewConnectHandler(useCase *connectNetwork.UseCase, owner workload.Owner) *connectHandler {
	return &connectHandler{useCase: useCase, owner: owner}
}

// @Summary		Connect a container to a network
// @Description	attach a container to another docker network of its VM, under the aliases its neighbours there reach it by
// @Tags			dashboard workload containers
// @Accept			json
// @Param			uuid	path		string					true	"VM UUID"
// @Param			id		path		string					true	"Container id or name"
// @Param			body	body		connectNetwork.Request	true	"The network"
// @Success		204		{object}	map[string]interface{}
// @Failure		400		{object}	workload.Refusal
// @Failure		404		{object}	workload.Failure
// @Failure		500		{object}	workload.Failure
// @Router			/dashboard/workload/vms/{uuid}/containers/{id}/networks [post]
// @Router			/dashboard/my/workload/vms/{uuid}/containers/{id}/networks [post]
func (h *connectHandler) ServeHTTP(rw http.ResponseWriter, r *http.Request) {
	var request connectNetwork.Request
	if !workload.Decode(rw, r, &request) {
		return
	}

	request.VMUUID = r.PathValue("uuid")
	request.ID = r.PathValue("id")
	request.OwnerUUID = h.owner(r)

	response, err := h.useCase.Execute(r.Context(), &request)

	switch {
	case workload.Failed(rw, r, err):
	case workload.Refused(rw, response.ValidationErrors):
	default:
		rw.WriteHeader(http.StatusNoContent)
	}
}
