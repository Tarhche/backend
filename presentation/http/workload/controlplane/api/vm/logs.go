package vm

import (
	"net/http"

	getvmlogs "github.com/khanzadimahdi/testproject/application/workload/controlplane/vm/getVMLogs"
	"github.com/khanzadimahdi/testproject/presentation/http/workload/controlplane/api/respond"
)

type logsHandler struct {
	useCase *getvmlogs.UseCase
}

func NewLogsHandler(useCase *getvmlogs.UseCase) *logsHandler {
	return &logsHandler{useCase: useCase}
}

// @Summary		Read a vm's log
// @Description	the tail of a vm's log, read from the node holding it as it is now
// @Tags			workload vms
// @Produce		json
// @Param			uuid	path		string	true	"VM UUID"
// @Param			owner	query		string	false	"Only this owner's vm"
// @Param			since	query		string	false	"Only lines written after this moment (RFC 3339)"
// @Param			tail	query		int		false	"Only the last lines"
// @Success		200		{object}	getvmlogs.Response
// @Failure		404		{object}	map[string]interface{}
// @Failure		409		{object}	map[string]interface{}
// @Router			/vms/{uuid}/logs [get]
func (h *logsHandler) ServeHTTP(rw http.ResponseWriter, r *http.Request) {
	response, err := h.useCase.Execute(r.Context(), &getvmlogs.Request{
		OwnerUUID: respond.Owner(r),
		UUID:      r.PathValue("uuid"),
		Since:     respond.Since(r),
		Tail:      respond.Tail(r),
	})

	switch {
	case err != nil:
		respond.Failed(rw, r, err)
	case len(response.ValidationErrors) > 0:
		respond.Refused(rw, response.ValidationErrors)
	case response.NodeError != nil:
		respond.NodeRefused(rw, response.NodeError)
	default:
		respond.JSON(rw, http.StatusOK, response)
	}
}
