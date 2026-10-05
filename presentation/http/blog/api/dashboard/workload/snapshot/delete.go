package snapshot

import (
	"net/http"

	"github.com/khanzadimahdi/testproject/application/dashboard/workload/snapshot/deleteSnapshot"
	"github.com/khanzadimahdi/testproject/presentation/http/blog/api/dashboard/workload"
)

type deleteHandler struct {
	useCase *deleteSnapshot.UseCase
	owner   workload.Owner
}

func NewDeleteHandler(useCase *deleteSnapshot.UseCase, owner workload.Owner) *deleteHandler {
	return &deleteHandler{useCase: useCase, owner: owner}
}

// @Summary		Delete a snapshot
// @Description	remove a snapshot and the archive it kept
// @Tags			dashboard workload snapshots
// @Param			uuid	path		string	true	"Snapshot UUID"
// @Success		204		{object}	map[string]interface{}
// @Failure		400		{object}	workload.Refusal
// @Failure		404		{object}	workload.Failure
// @Failure		500		{object}	workload.Failure
// @Router			/dashboard/workload/snapshots/{uuid} [delete]
// @Router			/dashboard/my/workload/snapshots/{uuid} [delete]
func (h *deleteHandler) ServeHTTP(rw http.ResponseWriter, r *http.Request) {
	response, err := h.useCase.Execute(r.Context(), &deleteSnapshot.Request{
		UUID:      r.PathValue("uuid"),
		OwnerUUID: h.owner(r),
	})

	switch {
	case workload.Failed(rw, r, err):
	case workload.Refused(rw, response.ValidationErrors):
	default:
		rw.WriteHeader(http.StatusNoContent)
	}
}
