package container

import (
	"net/http"

	"github.com/khanzadimahdi/testproject/application/dashboard/workload/container/startContainer"
	"github.com/khanzadimahdi/testproject/presentation/http/blog/api/dashboard/workload"
)

type startHandler struct {
	useCase *startContainer.UseCase
	owner   workload.Owner
}

func NewStartHandler(useCase *startContainer.UseCase, owner workload.Owner) *startHandler {
	return &startHandler{useCase: useCase, owner: owner}
}

// @Summary		Start a container
// @Description	start a container that is not running; it is running once this answers
// @Tags			dashboard workload containers
// @Param			uuid	path		string	true	"VM UUID"
// @Param			id		path		string	true	"Container id or name"
// @Success		204		{object}	map[string]interface{}
// @Failure		400		{object}	workload.Refusal
// @Failure		404		{object}	workload.Failure
// @Failure		500		{object}	workload.Failure
// @Router			/dashboard/workload/vms/{uuid}/containers/{id}/start [post]
// @Router			/dashboard/my/workload/vms/{uuid}/containers/{id}/start [post]
func (h *startHandler) ServeHTTP(rw http.ResponseWriter, r *http.Request) {
	response, err := h.useCase.Execute(r.Context(), &startContainer.Request{
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
