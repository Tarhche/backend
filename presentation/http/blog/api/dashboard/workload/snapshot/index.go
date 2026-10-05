package snapshot

import (
	"net/http"

	"github.com/khanzadimahdi/testproject/application/dashboard/workload/snapshot/getSnapshots"
	"github.com/khanzadimahdi/testproject/presentation/http/blog/api/dashboard/workload"
)

type indexHandler struct {
	useCase *getSnapshots.UseCase
	owner   workload.Owner
}

func NewIndexHandler(useCase *getSnapshots.UseCase, owner workload.Owner) *indexHandler {
	return &indexHandler{useCase: useCase, owner: owner}
}

// @Summary		List snapshots
// @Description	a page of snapshots, of one VM's with ?vm=: anybody's on the workload routes, the caller's own on the my routes
// @Tags			dashboard workload snapshots
// @Produce		json
// @Param			page	query		int		false	"Page"	default(1)
// @Param			vm		query		string	false	"Only the snapshots of this VM"
// @Success		200		{object}	getSnapshots.Response
// @Failure		500		{object}	map[string]interface{}
// @Router			/dashboard/workload/snapshots [get]
// @Router			/dashboard/my/workload/snapshots [get]
func (h *indexHandler) ServeHTTP(rw http.ResponseWriter, r *http.Request) {
	response, err := h.useCase.Execute(r.Context(), &getSnapshots.Request{
		Page:      workload.Page(r),
		VMUUID:    r.URL.Query().Get("vm"),
		OwnerUUID: h.owner(r),
	})

	if workload.Failed(rw, r, err) {
		return
	}

	workload.JSON(rw, http.StatusOK, response)
}
