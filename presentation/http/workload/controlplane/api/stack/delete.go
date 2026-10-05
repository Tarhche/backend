package stack

import (
	"net/http"

	deletestack "github.com/khanzadimahdi/testproject/application/workload/controlplane/stack/deleteStack"
	"github.com/khanzadimahdi/testproject/presentation/http/workload/controlplane/api/respond"
)

type deleteHandler struct {
	useCase *deletestack.UseCase
}

func NewDeleteHandler(useCase *deletestack.UseCase) *deleteHandler {
	return &deleteHandler{useCase: useCase}
}

// @Summary		Delete a stack
// @Description	take a stack down and remove it, its volumes too when asked
// @Tags			workload stacks
// @Param			uuid	path	string	true	"Stack UUID"
// @Param			owner	query	string	false	"Only this owner's stack"
// @Param			volumes	query	bool	false	"Remove the project's volumes too"
// @Success		202
// @Success		204
// @Failure		400	{object}	map[string]interface{}
// @Failure		404	{object}	map[string]interface{}
// @Router			/stacks/{uuid} [delete]
func (h *deleteHandler) ServeHTTP(rw http.ResponseWriter, r *http.Request) {
	response, err := h.useCase.Execute(r.Context(), &deletestack.Request{
		OwnerUUID:     respond.Owner(r),
		UUID:          r.PathValue("uuid"),
		RemoveVolumes: r.URL.Query().Get("volumes") == "true",
	})

	switch {
	case err != nil:
		respond.Failed(rw, r, err)
	case len(response.ValidationErrors) > 0:
		respond.Refused(rw, response.ValidationErrors)
	case response.Pending:
		rw.WriteHeader(http.StatusAccepted)
	default:
		rw.WriteHeader(http.StatusNoContent)
	}
}
