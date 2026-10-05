package vm

import (
	"net/http"

	updatevm "github.com/khanzadimahdi/testproject/application/workload/controlplane/vm/updateVM"
	"github.com/khanzadimahdi/testproject/presentation/http/workload/controlplane/api/respond"
)

type updateHandler struct {
	useCase *updatevm.UseCase
}

func NewUpdateHandler(useCase *updatevm.UseCase) *updateHandler {
	return &updateHandler{useCase: useCase}
}

// @Summary		Change a vm
// @Description	change a vm's name, lifetime, ports, network or resources; a change to the last three restarts it when it is running
// @Tags			workload vms
// @Accept			json
// @Produce		json
// @Param			uuid	path		string				true	"VM UUID"
// @Param			owner	query		string				false	"Only this owner's vm"
// @Param			body	body		updatevm.Request	true	"What changes"
// @Success		200		{object}	presenter.VM
// @Failure		400		{object}	map[string]interface{}
// @Failure		404		{object}	map[string]interface{}
// @Failure		500		{object}	map[string]interface{}
// @Router			/vms/{uuid} [patch]
func (h *updateHandler) ServeHTTP(rw http.ResponseWriter, r *http.Request) {
	var request updatevm.Request
	if !respond.Decode(rw, r, &request) {
		return
	}

	request.OwnerUUID = respond.Owner(r)
	request.UUID = r.PathValue("uuid")

	response, err := h.useCase.Execute(r.Context(), &request)
	switch {
	case err != nil:
		respond.Failed(rw, r, err)
	case len(response.ValidationErrors) > 0:
		respond.Refused(rw, response.ValidationErrors)
	default:
		respond.JSON(rw, http.StatusOK, response.VM)
	}
}
