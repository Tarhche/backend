package volume

import (
	"net/http"

	"github.com/khanzadimahdi/testproject/application/dashboard/workload/volume/getVolumeDependents"
	"github.com/khanzadimahdi/testproject/presentation/http/blog/api/dashboard/workload"
)

type dependentsHandler struct {
	useCase *getVolumeDependents.UseCase
	owner   workload.Owner
}

func NewDependentsHandler(useCase *getVolumeDependents.UseCase, owner workload.Owner) *dependentsHandler {
	return &dependentsHandler{useCase: useCase, owner: owner}
}

// @Summary		What needs a volume
// @Description	what needs a volume of a Docker VM, found by its name as its delete finds it, read before it is asked to be deleted: the containers and the stacks that need it, a container that mounts it among them, each saying whether it makes it again once it is gone, and whether one of them does, as its delete would be answered with them now; a list of none when nothing does. Reading it deletes nothing, and asks nothing of its node
// @Tags			dashboard workload volumes
// @Produce		json
// @Param			uuid	path		string	true	"VM UUID"
// @Param			name	path		string	true	"Volume name"
// @Success		200		{object}	presenter.Needs
// @Failure		400		{object}	workload.Refusal
// @Failure		404		{object}	workload.Failure
// @Failure		500		{object}	workload.Failure
// @Router			/dashboard/workload/vms/{uuid}/volumes/{name}/dependents [get]
// @Router			/dashboard/my/workload/vms/{uuid}/volumes/{name}/dependents [get]
func (h *dependentsHandler) ServeHTTP(rw http.ResponseWriter, r *http.Request) {
	response, err := h.useCase.Execute(r.Context(), &getVolumeDependents.Request{
		VMUUID:    r.PathValue("uuid"),
		Name:      r.PathValue("name"),
		OwnerUUID: h.owner(r),
	})

	switch {
	case workload.Failed(rw, r, err):
	case workload.Refused(rw, response.ValidationErrors):
	default:
		workload.JSON(rw, http.StatusOK, response.Needs)
	}
}
