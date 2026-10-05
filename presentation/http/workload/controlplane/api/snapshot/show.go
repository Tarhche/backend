package snapshot

import (
	"net/http"

	getsnapshot "github.com/khanzadimahdi/testproject/application/workload/controlplane/snapshot/getSnapshot"
	"github.com/khanzadimahdi/testproject/presentation/http/workload/controlplane/api/respond"
)

type showHandler struct {
	useCase *getsnapshot.UseCase
}

func NewShowHandler(useCase *getsnapshot.UseCase) *showHandler {
	return &showHandler{useCase: useCase}
}

// @Summary		Get a snapshot
// @Tags			workload snapshots
// @Produce		json
// @Param			uuid	path		string	true	"Snapshot UUID"
// @Param			owner	query		string	false	"Only this owner's snapshot"
// @Success		200		{object}	presenter.Snapshot
// @Failure		404		{object}	map[string]interface{}
// @Router			/snapshots/{uuid} [get]
func (h *showHandler) ServeHTTP(rw http.ResponseWriter, r *http.Request) {
	response, err := h.useCase.Execute(r.Context(), &getsnapshot.Request{OwnerUUID: respond.Owner(r), UUID: r.PathValue("uuid")})
	if err != nil {
		respond.Failed(rw, r, err)

		return
	}

	respond.JSON(rw, http.StatusOK, response)
}
