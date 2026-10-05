package vm

import (
	"net/http"

	getvms "github.com/khanzadimahdi/testproject/application/workload/controlplane/vm/getVMs"
	domainVM "github.com/khanzadimahdi/testproject/domain/workload/vm"
	"github.com/khanzadimahdi/testproject/presentation/http/workload/controlplane/api/respond"
)

type indexHandler struct {
	useCase *getvms.UseCase
}

func NewIndexHandler(useCase *getvms.UseCase) *indexHandler {
	return &indexHandler{useCase: useCase}
}

// @Summary		List vms
// @Description	return a page of vms
// @Tags			workload vms
// @Produce		json
// @Param			owner	query		string	false	"Only the vms this person owns"
// @Param			kind	query		string	false	"Only the vms of this kind: machine or docker"
// @Param			page	query		int		false	"Page number"	default(1)
// @Success		200		{object}	getvms.Response
// @Failure		500		{object}	map[string]interface{}
// @Router			/vms [get]
func (h *indexHandler) ServeHTTP(rw http.ResponseWriter, r *http.Request) {
	response, err := h.useCase.Execute(r.Context(), &getvms.Request{
		OwnerUUID: respond.Owner(r),
		Kind:      domainVM.Kind(r.URL.Query().Get("kind")),
		Page:      respond.Page(r),
	})
	if err != nil {
		respond.Failed(rw, r, err)

		return
	}

	respond.JSON(rw, http.StatusOK, response)
}
