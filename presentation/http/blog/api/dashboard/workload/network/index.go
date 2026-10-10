package network

import (
	"net/http"

	"github.com/khanzadimahdi/testproject/application/dashboard/workload/network/getNetworks"
	"github.com/khanzadimahdi/testproject/presentation/http/blog/api/dashboard/workload"
)

type indexHandler struct {
	useCase *getNetworks.UseCase
	owner   workload.Owner
}

func NewIndexHandler(useCase *getNetworks.UseCase, owner workload.Owner) *indexHandler {
	return &indexHandler{useCase: useCase, owner: owner}
}

// @Summary		List a Docker VM's networks
// @Description	the docker networks of a Docker VM, the ones docker made itself included
// @Tags			dashboard workload networks
// @Produce		json
// @Param			uuid	path		string	true	"VM UUID"
// @Success		200		{object}	getNetworks.Response
// @Failure		400		{object}	workload.Refusal
// @Failure		404		{object}	workload.Failure
// @Failure		500		{object}	workload.Failure
// @Router			/dashboard/workload/vms/{uuid}/networks [get]
// @Router			/dashboard/my/workload/vms/{uuid}/networks [get]
func (h *indexHandler) ServeHTTP(rw http.ResponseWriter, r *http.Request) {
	response, err := h.useCase.Execute(r.Context(), &getNetworks.Request{
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
