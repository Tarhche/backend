package stack

import (
	"net/http"

	"github.com/khanzadimahdi/testproject/application/dashboard/workload/stack/deleteStack"
	"github.com/khanzadimahdi/testproject/presentation/http/blog/api/dashboard/workload"
)

type deleteHandler struct {
	useCase *deleteStack.UseCase
	owner   workload.Owner
}

func NewDeleteHandler(useCase *deleteStack.UseCase, owner workload.Owner) *deleteHandler {
	return &deleteHandler{useCase: useCase, owner: owner}
}

// @Summary		Delete a stack
// @Description	take a stack down and remove it, with its volumes when ?volumes=true; it is removing until compose is done
// @Tags			dashboard workload stacks
// @Param			uuid	path		string	true	"Stack UUID"
// @Param			volumes	query		bool	false	"Remove its volumes too"
// @Success		202		{object}	map[string]interface{}
// @Failure		400		{object}	workload.Refusal
// @Failure		404		{object}	map[string]interface{}
// @Failure		500		{object}	map[string]interface{}
// @Router			/dashboard/workload/stacks/{uuid} [delete]
// @Router			/dashboard/my/workload/stacks/{uuid} [delete]
func (h *deleteHandler) ServeHTTP(rw http.ResponseWriter, r *http.Request) {
	response, err := h.useCase.Execute(r.Context(), &deleteStack.Request{
		UUID:          r.PathValue("uuid"),
		RemoveVolumes: workload.Flag(r, "volumes"),
		OwnerUUID:     h.owner(r),
	})

	switch {
	case workload.Failed(rw, r, err):
	case workload.Refused(rw, response.ValidationErrors):
	default:
		rw.WriteHeader(http.StatusAccepted)
	}
}
