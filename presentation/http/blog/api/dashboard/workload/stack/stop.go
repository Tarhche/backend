package stack

import (
	"net/http"

	"github.com/khanzadimahdi/testproject/application/dashboard/workload/stack/stopStack"
	"github.com/khanzadimahdi/testproject/presentation/http/blog/api/dashboard/workload"
)

type stopHandler struct {
	useCase *stopStack.UseCase
	owner   workload.Owner
}

func NewStopHandler(useCase *stopStack.UseCase, owner workload.Owner) *stopHandler {
	return &stopHandler{useCase: useCase, owner: owner}
}

// @Summary		Stop a stack
// @Description	ask for a stack's containers to be stopped, and kept
// @Tags			dashboard workload stacks
// @Param			uuid	path		string	true	"Stack UUID"
// @Success		202		{object}	map[string]interface{}
// @Failure		400		{object}	workload.Refusal
// @Failure		404		{object}	map[string]interface{}
// @Failure		500		{object}	map[string]interface{}
// @Router			/dashboard/workload/stacks/{uuid}/stop [post]
// @Router			/dashboard/my/workload/stacks/{uuid}/stop [post]
func (h *stopHandler) ServeHTTP(rw http.ResponseWriter, r *http.Request) {
	response, err := h.useCase.Execute(r.Context(), &stopStack.Request{
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
