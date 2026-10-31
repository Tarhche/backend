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
// @Description	take a stack down and remove it; it is removing until its graph has removed its containers and its networks. Its volumes stay, and so does its VM
// @Tags			dashboard workload stacks
// @Param			uuid	path		string	true	"Stack UUID"
// @Success		202		{object}	map[string]interface{}
// @Failure		400		{object}	workload.Refusal
// @Failure		404		{object}	workload.Failure
// @Failure		500		{object}	workload.Failure
// @Router			/dashboard/workload/stacks/{uuid} [delete]
// @Router			/dashboard/my/workload/stacks/{uuid} [delete]
func (h *deleteHandler) ServeHTTP(rw http.ResponseWriter, r *http.Request) {
	response, err := h.useCase.Execute(r.Context(), &deleteStack.Request{
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
