package container

import (
	"net/http"

	"github.com/khanzadimahdi/testproject/application/dashboard/workload/container/getContainer"
	"github.com/khanzadimahdi/testproject/presentation/http/blog/api/dashboard/workload"
)

type showHandler struct {
	useCase *getContainer.UseCase
	owner   workload.Owner
}

func NewShowHandler(useCase *getContainer.UseCase, owner workload.Owner) *showHandler {
	return &showHandler{useCase: useCase, owner: owner}
}

// @Summary		Show a container
// @Description	one container, read from its VM's dockerd as it is now
// @Tags			dashboard workload containers
// @Produce		json
// @Param			uuid	path		string	true	"VM UUID"
// @Param			id		path		string	true	"Container id or name"
// @Success		200		{object}	presenter.Container
// @Failure		400		{object}	workload.Refusal
// @Failure		404		{object}	workload.Failure
// @Failure		500		{object}	workload.Failure
// @Router			/dashboard/workload/vms/{uuid}/containers/{id} [get]
// @Router			/dashboard/my/workload/vms/{uuid}/containers/{id} [get]
func (h *showHandler) ServeHTTP(rw http.ResponseWriter, r *http.Request) {
	response, err := h.useCase.Execute(r.Context(), &getContainer.Request{
		VMUUID:    r.PathValue("uuid"),
		ID:        r.PathValue("id"),
		OwnerUUID: h.owner(r),
	})

	switch {
	case workload.Failed(rw, r, err):
	case workload.Refused(rw, response.ValidationErrors):
	default:
		workload.JSON(rw, http.StatusOK, response.Container)
	}
}
