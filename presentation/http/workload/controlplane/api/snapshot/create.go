package snapshot

import (
	"net/http"

	createsnapshot "github.com/khanzadimahdi/testproject/application/workload/controlplane/snapshot/createSnapshot"
	"github.com/khanzadimahdi/testproject/presentation/http/workload/controlplane/api/respond"
)

type createHandler struct {
	useCase *createsnapshot.UseCase
}

func NewCreateHandler(useCase *createsnapshot.UseCase) *createHandler {
	return &createHandler{useCase: useCase}
}

// @Summary		Take a snapshot of a vm
// @Tags			workload snapshots
// @Accept			json
// @Produce		json
// @Param			uuid	path		string					true	"VM UUID"
// @Param			owner	query		string					false	"Only this owner's vm"
// @Param			body	body		createsnapshot.Request	true	"The snapshot"
// @Success		201		{object}	presenter.Snapshot
// @Failure		400		{object}	map[string]interface{}
// @Failure		404		{object}	map[string]interface{}
// @Router			/vms/{uuid}/snapshots [post]
func (h *createHandler) ServeHTTP(rw http.ResponseWriter, r *http.Request) {
	var request createsnapshot.Request
	if !respond.Decode(rw, r, &request) {
		return
	}

	request.OwnerUUID = respond.Owner(r)
	request.VMUUID = r.PathValue("uuid")

	response, err := h.useCase.Execute(r.Context(), &request)
	switch {
	case err != nil:
		respond.Failed(rw, r, err)
	case len(response.ValidationErrors) > 0:
		respond.Refused(rw, response.ValidationErrors)
	default:
		respond.JSON(rw, http.StatusCreated, response.Snapshot)
	}
}
