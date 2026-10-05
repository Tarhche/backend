package snapshot

import (
	"net/http"

	renamesnapshot "github.com/khanzadimahdi/testproject/application/workload/controlplane/snapshot/renameSnapshot"
	"github.com/khanzadimahdi/testproject/presentation/http/workload/controlplane/api/respond"
)

type renameHandler struct {
	useCase *renamesnapshot.UseCase
}

func NewRenameHandler(useCase *renamesnapshot.UseCase) *renameHandler {
	return &renameHandler{useCase: useCase}
}

// @Summary		Rename a snapshot
// @Tags			workload snapshots
// @Accept			json
// @Produce		json
// @Param			uuid	path		string					true	"Snapshot UUID"
// @Param			owner	query		string					false	"Only this owner's snapshot"
// @Param			body	body		renamesnapshot.Request	true	"The new name"
// @Success		200		{object}	presenter.Snapshot
// @Failure		400		{object}	map[string]interface{}
// @Failure		404		{object}	map[string]interface{}
// @Router			/snapshots/{uuid} [patch]
func (h *renameHandler) ServeHTTP(rw http.ResponseWriter, r *http.Request) {
	var request renamesnapshot.Request
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
		respond.JSON(rw, http.StatusOK, response.Snapshot)
	}
}
