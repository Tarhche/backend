package vm

import (
	"net/http"

	restorevm "github.com/khanzadimahdi/testproject/application/workload/controlplane/vm/restoreVM"
	"github.com/khanzadimahdi/testproject/presentation/http/workload/controlplane/api/respond"
)

type restoreHandler struct {
	useCase *restorevm.UseCase
}

func NewRestoreHandler(useCase *restorevm.UseCase) *restoreHandler {
	return &restoreHandler{useCase: useCase}
}

// @Summary		Restore a vm from a snapshot
// @Description	replace a vm's disk from a snapshot of the same owner, kind and engine; the vm keeps its uuid, slug and ports
// @Tags			workload vms
// @Accept			json
// @Param			uuid	path	string				true	"VM UUID"
// @Param			owner	query	string				false	"Only this owner's vm"
// @Param			body	body	restorevm.Request	true	"The snapshot"
// @Success		202
// @Failure		400	{object}	map[string]interface{}
// @Failure		404	{object}	map[string]interface{}
// @Router			/vms/{uuid}/restore [post]
func (h *restoreHandler) ServeHTTP(rw http.ResponseWriter, r *http.Request) {
	var request restorevm.Request
	if !respond.Decode(rw, r, &request) {
		return
	}

	request.OwnerUUID = respond.Owner(r)
	request.UUID = r.PathValue("uuid")

	response, err := h.useCase.Execute(r.Context(), &request)
	if err != nil {
		asked(rw, r, nil, err)

		return
	}

	asked(rw, r, response.ValidationErrors, nil)
}
