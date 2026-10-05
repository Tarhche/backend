package stack

import (
	"net/http"

	"github.com/khanzadimahdi/testproject/application/dashboard/workload/stack/getStack"
	"github.com/khanzadimahdi/testproject/presentation/http/blog/api/dashboard/workload"
)

type showHandler struct {
	useCase *getStack.UseCase
	owner   workload.Owner
}

func NewShowHandler(useCase *getStack.UseCase, owner workload.Owner) *showHandler {
	return &showHandler{useCase: useCase, owner: owner}
}

// @Summary		Show a stack
// @Description	one stack, with its compose file and the containers compose made for it as its VM lists them now; note is vm_not_running when there are none because its VM is not running
// @Tags			dashboard workload stacks
// @Produce		json
// @Param			uuid	path		string	true	"Stack UUID"
// @Success		200		{object}	getStack.Response
// @Failure		404		{object}	workload.Failure
// @Failure		500		{object}	workload.Failure
// @Router			/dashboard/workload/stacks/{uuid} [get]
// @Router			/dashboard/my/workload/stacks/{uuid} [get]
func (h *showHandler) ServeHTTP(rw http.ResponseWriter, r *http.Request) {
	response, err := h.useCase.Execute(r.Context(), &getStack.Request{
		UUID:      r.PathValue("uuid"),
		OwnerUUID: h.owner(r),
	})

	if workload.Failed(rw, r, err) {
		return
	}

	workload.JSON(rw, http.StatusOK, response)
}
