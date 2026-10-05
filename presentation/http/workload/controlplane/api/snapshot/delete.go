package snapshot

import (
	"net/http"

	deletesnapshot "github.com/khanzadimahdi/testproject/application/workload/controlplane/snapshot/deleteSnapshot"
	"github.com/khanzadimahdi/testproject/presentation/http/workload/controlplane/api/respond"
)

type deleteHandler struct {
	useCase *deletesnapshot.UseCase
}

func NewDeleteHandler(useCase *deletesnapshot.UseCase) *deleteHandler {
	return &deleteHandler{useCase: useCase}
}

// @Summary		Delete a snapshot
// @Description	take a snapshot's archive away and its record; one still being taken goes once its node has finished with it
// @Tags			workload snapshots
// @Param			uuid	path	string	true	"Snapshot UUID"
// @Param			owner	query	string	false	"Only this owner's snapshot"
// @Success		204
// @Success		202
// @Failure		404	{object}	map[string]interface{}
// @Router			/snapshots/{uuid} [delete]
func (h *deleteHandler) ServeHTTP(rw http.ResponseWriter, r *http.Request) {
	response, err := h.useCase.Execute(r.Context(), &deletesnapshot.Request{OwnerUUID: respond.Owner(r), UUID: r.PathValue("uuid")})
	switch {
	case err != nil:
		respond.Failed(rw, r, err)
	case len(response.ValidationErrors) > 0:
		respond.Refused(rw, response.ValidationErrors)
	case response.Pending:
		rw.WriteHeader(http.StatusAccepted)
	default:
		rw.WriteHeader(http.StatusNoContent)
	}
}
