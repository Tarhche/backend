package snapshot

import (
	"net/http"

	"github.com/khanzadimahdi/testproject/application/dashboard/workload/snapshot/createSnapshot"
	"github.com/khanzadimahdi/testproject/presentation/http/blog/api/dashboard/workload"
)

type createHandler struct {
	useCase *createSnapshot.UseCase
}

func NewCreateHandler(useCase *createSnapshot.UseCase) *createHandler {
	return &createHandler{useCase: useCase}
}

// @Summary		Take a snapshot
// @Description	take a snapshot of one of the caller's own VMs, which is running or stopped; it is being created until its archive is stored
// @Tags			dashboard workload snapshots
// @Accept			json
// @Produce		json
// @Param			uuid	path		string					true	"VM UUID"
// @Param			body	body		createSnapshot.Request	true	"Snapshot"
// @Success		201		{object}	presenter.Snapshot
// @Failure		400		{object}	workload.Refusal
// @Failure		404		{object}	map[string]interface{}
// @Failure		500		{object}	map[string]interface{}
// @Router			/dashboard/workload/vms/{uuid}/snapshots [post]
func (h *createHandler) ServeHTTP(rw http.ResponseWriter, r *http.Request) {
	var request createSnapshot.Request
	if !workload.Decode(rw, r, &request) {
		return
	}

	request.VMUUID = r.PathValue("uuid")
	request.OwnerUUID = workload.Caller(r)

	response, err := h.useCase.Execute(r.Context(), &request)

	switch {
	case workload.Failed(rw, r, err):
	case workload.Refused(rw, response.ValidationErrors):
	default:
		workload.JSON(rw, http.StatusCreated, response.Snapshot)
	}
}
