package vm

import (
	"net/http"

	deletevm "github.com/khanzadimahdi/testproject/application/workload/controlplane/vm/deleteVM"
	"github.com/khanzadimahdi/testproject/presentation/http/workload/controlplane/api/respond"
)

type deleteHandler struct {
	useCase *deletevm.UseCase
}

func NewDeleteHandler(useCase *deletevm.UseCase) *deleteHandler {
	return &deleteHandler{useCase: useCase}
}

// @Summary		Delete a vm
// @Description	ask the node holding a vm to remove it; its record goes once the node confirms, and its snapshots stay
// @Tags			workload vms
// @Param			uuid	path	string	true	"VM UUID"
// @Param			owner	query	string	false	"Only this owner's vm"
// @Success		202
// @Failure		404	{object}	map[string]interface{}
// @Failure		500	{object}	map[string]interface{}
// @Router			/vms/{uuid} [delete]
func (h *deleteHandler) ServeHTTP(rw http.ResponseWriter, r *http.Request) {
	response, err := h.useCase.Execute(r.Context(), &deletevm.Request{OwnerUUID: respond.Owner(r), UUID: r.PathValue("uuid")})
	switch {
	case err != nil:
		respond.Failed(rw, r, err)
	case len(response.ValidationErrors) > 0:
		respond.Refused(rw, response.ValidationErrors)
	default:
		rw.WriteHeader(http.StatusAccepted)
	}
}
