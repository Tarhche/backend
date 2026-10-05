package snapshot

import (
	"net/http"

	"github.com/khanzadimahdi/testproject/application/dashboard/workload/snapshot/getSnapshot"
	"github.com/khanzadimahdi/testproject/presentation/http/blog/api/dashboard/workload"
)

type showHandler struct {
	useCase *getSnapshot.UseCase
	owner   workload.Owner
}

func NewShowHandler(useCase *getSnapshot.UseCase, owner workload.Owner) *showHandler {
	return &showHandler{useCase: useCase, owner: owner}
}

// @Summary		Show a snapshot
// @Description	one snapshot
// @Tags			dashboard workload snapshots
// @Produce		json
// @Param			uuid	path		string	true	"Snapshot UUID"
// @Success		200		{object}	getSnapshot.Response
// @Failure		404		{object}	workload.Failure
// @Failure		500		{object}	workload.Failure
// @Router			/dashboard/workload/snapshots/{uuid} [get]
// @Router			/dashboard/my/workload/snapshots/{uuid} [get]
func (h *showHandler) ServeHTTP(rw http.ResponseWriter, r *http.Request) {
	response, err := h.useCase.Execute(r.Context(), &getSnapshot.Request{
		UUID:      r.PathValue("uuid"),
		OwnerUUID: h.owner(r),
	})

	if workload.Failed(rw, r, err) {
		return
	}

	workload.JSON(rw, http.StatusOK, response)
}
