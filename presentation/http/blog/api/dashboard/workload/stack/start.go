package stack

import (
	"net/http"

	"github.com/khanzadimahdi/testproject/application/dashboard/workload/stack/startStack"
	"github.com/khanzadimahdi/testproject/presentation/http/blog/api/dashboard/workload"
)

type startHandler struct {
	useCase *startStack.UseCase
	owner   workload.Owner
}

func NewStartHandler(useCase *startStack.UseCase, owner workload.Owner) *startHandler {
	return &startHandler{useCase: useCase, owner: owner}
}

// @Summary		Start a stack
// @Description	ask for a stopped stack's containers to be started again; its state says how it went
// @Tags			dashboard workload stacks
// @Param			uuid	path		string	true	"Stack UUID"
// @Success		202		{object}	map[string]interface{}
// @Failure		400		{object}	workload.Refusal
// @Failure		404		{object}	workload.Failure
// @Failure		500		{object}	workload.Failure
// @Router			/dashboard/workload/stacks/{uuid}/start [post]
// @Router			/dashboard/my/workload/stacks/{uuid}/start [post]
func (h *startHandler) ServeHTTP(rw http.ResponseWriter, r *http.Request) {
	response, err := h.useCase.Execute(r.Context(), &startStack.Request{
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
