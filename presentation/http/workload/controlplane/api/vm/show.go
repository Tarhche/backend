package vm

import (
	"net/http"

	getvm "github.com/khanzadimahdi/testproject/application/workload/controlplane/vm/getVM"
	"github.com/khanzadimahdi/testproject/presentation/http/workload/controlplane/api/respond"
)

type showHandler struct {
	useCase *getvm.UseCase
}

func NewShowHandler(useCase *getvm.UseCase) *showHandler {
	return &showHandler{useCase: useCase}
}

// @Summary		Get a vm
// @Tags			workload vms
// @Produce		json
// @Param			uuid	path		string	true	"VM UUID"
// @Param			owner	query		string	false	"Only this owner's vm"
// @Success		200		{object}	presenter.VM
// @Failure		404		{object}	map[string]interface{}
// @Failure		500		{object}	map[string]interface{}
// @Router			/vms/{uuid} [get]
func (h *showHandler) ServeHTTP(rw http.ResponseWriter, r *http.Request) {
	response, err := h.useCase.Execute(r.Context(), &getvm.Request{OwnerUUID: respond.Owner(r), UUID: r.PathValue("uuid")})
	if err != nil {
		respond.Failed(rw, r, err)

		return
	}

	respond.JSON(rw, http.StatusOK, response)
}
