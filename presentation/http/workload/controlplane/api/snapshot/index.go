package snapshot

import (
	"net/http"

	getsnapshots "github.com/khanzadimahdi/testproject/application/workload/controlplane/snapshot/getSnapshots"
	"github.com/khanzadimahdi/testproject/presentation/http/workload/controlplane/api/respond"
)

type indexHandler struct {
	useCase *getsnapshots.UseCase
}

func NewIndexHandler(useCase *getsnapshots.UseCase) *indexHandler {
	return &indexHandler{useCase: useCase}
}

// @Summary		List snapshots
// @Tags			workload snapshots
// @Produce		json
// @Param			owner	query		string	false	"Only the snapshots this person owns"
// @Param			vm		query		string	false	"Only the snapshots taken of this vm"
// @Param			page	query		int		false	"Page number"	default(1)
// @Success		200		{object}	getsnapshots.Response
// @Router			/snapshots [get]
func (h *indexHandler) ServeHTTP(rw http.ResponseWriter, r *http.Request) {
	response, err := h.useCase.Execute(r.Context(), &getsnapshots.Request{
		OwnerUUID: respond.Owner(r),
		VMUUID:    r.URL.Query().Get("vm"),
		Page:      respond.Page(r),
	})
	if err != nil {
		respond.Failed(rw, r, err)

		return
	}

	respond.JSON(rw, http.StatusOK, response)
}
