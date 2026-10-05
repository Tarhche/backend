package container

import (
	"net/http"

	"github.com/khanzadimahdi/testproject/application/dashboard/workload/container/getContainerStats"
	"github.com/khanzadimahdi/testproject/presentation/http/blog/api/dashboard/workload"
)

type statsHandler struct {
	useCase *getContainerStats.UseCase
	owner   workload.Owner
}

func NewStatsHandler(useCase *getContainerStats.UseCase, owner workload.Owner) *statsHandler {
	return &statsHandler{useCase: useCase, owner: owner}
}

// @Summary		Container stats
// @Description	one sample of what a container uses: CPU, memory, network and block counters, in bytes
// @Tags			dashboard workload containers
// @Produce		json
// @Param			uuid	path		string	true	"VM UUID"
// @Param			id		path		string	true	"Container id or name"
// @Success		200		{object}	presenter.ContainerStats
// @Failure		400		{object}	workload.Refusal
// @Failure		404		{object}	map[string]interface{}
// @Failure		500		{object}	map[string]interface{}
// @Router			/dashboard/workload/vms/{uuid}/containers/{id}/stats [get]
// @Router			/dashboard/my/workload/vms/{uuid}/containers/{id}/stats [get]
func (h *statsHandler) ServeHTTP(rw http.ResponseWriter, r *http.Request) {
	response, err := h.useCase.Execute(r.Context(), &getContainerStats.Request{
		VMUUID:    r.PathValue("uuid"),
		ID:        r.PathValue("id"),
		OwnerUUID: h.owner(r),
	})

	switch {
	case workload.Failed(rw, r, err):
	case workload.Refused(rw, response.ValidationErrors):
	default:
		workload.JSON(rw, http.StatusOK, response.ContainerStats)
	}
}
