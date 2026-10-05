package vm

import (
	"net/http"

	"github.com/khanzadimahdi/testproject/application/dashboard/workload/vm/getVMLogs"
	"github.com/khanzadimahdi/testproject/presentation/http/blog/api/dashboard/workload"
)

type logsHandler struct {
	useCase *getVMLogs.UseCase
	owner   workload.Owner
}

func NewLogsHandler(useCase *getVMLogs.UseCase, owner workload.Owner) *logsHandler {
	return &logsHandler{useCase: useCase, owner: owner}
}

// @Summary		VM logs
// @Description	the tail of what a VM has written, read from its node as it is now: the last tail lines, or those written since a moment
// @Tags			dashboard workload vms
// @Produce		json
// @Param			uuid	path		string	true	"VM UUID"
// @Param			since	query		string	false	"Only lines written from this moment on (RFC 3339)"
// @Param			tail	query		int		false	"Only the last this many lines"
// @Success		200		{object}	getVMLogs.Response
// @Failure		400		{object}	workload.Refusal
// @Failure		404		{object}	workload.Failure
// @Failure		500		{object}	workload.Failure
// @Router			/dashboard/workload/vms/{uuid}/logs [get]
// @Router			/dashboard/my/workload/vms/{uuid}/logs [get]
func (h *logsHandler) ServeHTTP(rw http.ResponseWriter, r *http.Request) {
	response, err := h.useCase.Execute(r.Context(), &getVMLogs.Request{
		UUID:      r.PathValue("uuid"),
		OwnerUUID: h.owner(r),
		Since:     workload.Since(r),
		Tail:      workload.Tail(r),
	})

	switch {
	case workload.Failed(rw, r, err):
	case workload.Refused(rw, response.ValidationErrors):
	default:
		workload.JSON(rw, http.StatusOK, response)
	}
}
