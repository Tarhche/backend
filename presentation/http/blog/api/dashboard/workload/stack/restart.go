package stack

import (
	"net/http"

	"github.com/khanzadimahdi/testproject/application/dashboard/workload/stack/restartStack"
	"github.com/khanzadimahdi/testproject/presentation/http/blog/api/dashboard/workload"
)

type restartHandler struct {
	useCase *restartStack.UseCase
	owner   workload.Owner
}

func NewRestartHandler(useCase *restartStack.UseCase, owner workload.Owner) *restartHandler {
	return &restartHandler{useCase: useCase, owner: owner}
}

// @Summary		Restart a stack
// @Description	ask for a stack's containers to be restarted
// @Tags			dashboard workload stacks
// @Param			uuid	path		string	true	"Stack UUID"
// @Success		202		{object}	map[string]interface{}
// @Failure		400		{object}	workload.Refusal
// @Failure		404		{object}	workload.Failure
// @Failure		500		{object}	workload.Failure
// @Router			/dashboard/workload/stacks/{uuid}/restart [post]
// @Router			/dashboard/my/workload/stacks/{uuid}/restart [post]
func (h *restartHandler) ServeHTTP(rw http.ResponseWriter, r *http.Request) {
	response, err := h.useCase.Execute(r.Context(), &restartStack.Request{
		UUID:      r.PathValue("uuid"),
		OwnerUUID: h.owner(r),
	})

	switch {
	case workload.Failed(rw, r, err):
	case workload.Refused(rw, response.ValidationErrors):
	default:
		rw.WriteHeader(http.StatusAccepted)
	}
}
