package network

import (
	"net/http"

	"github.com/khanzadimahdi/testproject/application/dashboard/workload/network/createNetwork"
	"github.com/khanzadimahdi/testproject/presentation/http/blog/api/dashboard/workload"
)

type createHandler struct {
	useCase *createNetwork.UseCase
	owner   workload.Owner
}

func NewCreateHandler(useCase *createNetwork.UseCase, owner workload.Owner) *createHandler {
	return &createHandler{useCase: useCase, owner: owner}
}

// @Summary		Create a network
// @Description	create a docker network in a Docker VM, for its containers to meet on; it never reaches past the VM
// @Tags			dashboard workload networks
// @Accept			json
// @Produce		json
// @Param			uuid	path		string					true	"VM UUID"
// @Param			body	body		createNetwork.Request	true	"Network"
// @Success		201		{object}	presenter.DockerNetwork
// @Failure		400		{object}	workload.Refusal
// @Failure		404		{object}	workload.Failure
// @Failure		500		{object}	workload.Failure
// @Router			/dashboard/workload/vms/{uuid}/networks [post]
// @Router			/dashboard/my/workload/vms/{uuid}/networks [post]
func (h *createHandler) ServeHTTP(rw http.ResponseWriter, r *http.Request) {
	var request createNetwork.Request
	if !workload.Decode(rw, r, &request) {
		return
	}

	request.VMUUID = r.PathValue("uuid")
	request.OwnerUUID = h.owner(r)

	response, err := h.useCase.Execute(r.Context(), &request)

	switch {
	case workload.Failed(rw, r, err):
	case workload.Refused(rw, response.ValidationErrors):
	default:
		workload.JSON(rw, http.StatusCreated, response.DockerNetwork)
	}
}
