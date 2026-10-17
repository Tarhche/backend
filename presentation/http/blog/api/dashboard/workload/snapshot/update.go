package snapshot

import (
	"net/http"

	"github.com/khanzadimahdi/testproject/application/dashboard/workload/snapshot/renameSnapshot"
	"github.com/khanzadimahdi/testproject/presentation/http/blog/api/dashboard/workload"
)

type updateHandler struct {
	useCase *renameSnapshot.UseCase
	owner   workload.Owner
}

func NewUpdateHandler(useCase *renameSnapshot.UseCase, owner workload.Owner) *updateHandler {
	return &updateHandler{useCase: useCase, owner: owner}
}

// @Summary		Rename a snapshot
// @Description	give a snapshot another name, which is all there is to change about one
// @Tags			dashboard workload snapshots
// @Accept			json
// @Produce		json
// @Param			uuid	path		string					true	"Snapshot UUID"
// @Param			body	body		renameSnapshot.Request	true	"Its name"
// @Success		200		{object}	presenter.Snapshot
// @Failure		400		{object}	workload.Refusal
// @Failure		404		{object}	workload.Failure
// @Failure		500		{object}	workload.Failure
// @Router			/dashboard/workload/snapshots/{uuid} [patch]
// @Router			/dashboard/my/workload/snapshots/{uuid} [patch]
func (h *updateHandler) ServeHTTP(rw http.ResponseWriter, r *http.Request) {
	var request renameSnapshot.Request
	if !workload.Decode(rw, r, &request) {
		return
	}

	request.UUID = r.PathValue("uuid")
	request.OwnerUUID = h.owner(r)

	response, err := h.useCase.Execute(r.Context(), &request)

	switch {
	case workload.Failed(rw, r, err):
	case workload.Refused(rw, response.ValidationErrors):
	default:
		workload.JSON(rw, http.StatusOK, response.Snapshot)
	}
}
