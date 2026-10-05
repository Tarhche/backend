package vm

import (
	"net/http"

	"github.com/khanzadimahdi/testproject/application/dashboard/workload/vm/createVM"
	"github.com/khanzadimahdi/testproject/presentation/http/blog/api/dashboard/workload"
)

type createHandler struct {
	useCase *createVM.UseCase
}

func NewCreateHandler(useCase *createVM.UseCase) *createHandler {
	return &createHandler{useCase: useCase}
}

// @Summary		Create a VM
// @Description	create a VM for the caller, from an image or from one of their snapshots; the workload boots it in its own time
// @Tags			dashboard workload vms
// @Accept			json
// @Produce		json
// @Param			body	body		createVM.Request	true	"VM"
// @Success		201		{object}	presenter.VM
// @Failure		400		{object}	workload.Refusal
// @Failure		500		{object}	map[string]interface{}
// @Router			/dashboard/workload/vms [post]
func (h *createHandler) ServeHTTP(rw http.ResponseWriter, r *http.Request) {
	var request createVM.Request
	if !workload.Decode(rw, r, &request) {
		return
	}

	request.OwnerUUID = workload.Caller(r)

	response, err := h.useCase.Execute(r.Context(), &request)

	switch {
	case workload.Failed(rw, r, err):
	case workload.Refused(rw, response.ValidationErrors):
	default:
		workload.JSON(rw, http.StatusCreated, response.VM)
	}
}
