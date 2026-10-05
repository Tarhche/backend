package vm

import (
	"net/http"

	createvm "github.com/khanzadimahdi/testproject/application/workload/controlplane/vm/createVM"
	"github.com/khanzadimahdi/testproject/presentation/http/workload/controlplane/api/respond"
)

type createHandler struct {
	useCase *createvm.UseCase
}

func NewCreateHandler(useCase *createvm.UseCase) *createHandler {
	return &createHandler{useCase: useCase}
}

// @Summary		Create a vm
// @Description	create a vm for the owner and ask a node to make it
// @Tags			workload vms
// @Accept			json
// @Produce		json
// @Param			owner	query		string				true	"Whom the vm is created for"
// @Param			body	body		createvm.Request	true	"The vm"
// @Success		201		{object}	presenter.VM
// @Failure		400		{object}	map[string]interface{}
// @Failure		500		{object}	map[string]interface{}
// @Router			/vms [post]
func (h *createHandler) ServeHTTP(rw http.ResponseWriter, r *http.Request) {
	var request createvm.Request
	if !respond.Decode(rw, r, &request) {
		return
	}

	request.OwnerUUID = respond.Owner(r)

	response, err := h.useCase.Execute(r.Context(), &request)
	switch {
	case err != nil:
		respond.Failed(rw, r, err)
	case len(response.ValidationErrors) > 0:
		respond.Refused(rw, response.ValidationErrors)
	default:
		respond.JSON(rw, http.StatusCreated, response.VM)
	}
}
