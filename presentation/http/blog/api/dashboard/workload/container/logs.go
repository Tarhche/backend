package container

import (
	"net/http"

	"github.com/khanzadimahdi/testproject/application/dashboard/workload/container/getContainerLogs"
	"github.com/khanzadimahdi/testproject/presentation/http/blog/api/dashboard/workload"
)

type logsHandler struct {
	useCase *getContainerLogs.UseCase
	owner   workload.Owner
}

func NewLogsHandler(useCase *getContainerLogs.UseCase, owner workload.Owner) *logsHandler {
	return &logsHandler{useCase: useCase, owner: owner}
}

// @Summary		Container logs
// @Description	the tail of what a container has written: the last tail lines, or those written since a moment
// @Tags			dashboard workload containers
// @Produce		json
// @Param			uuid	path		string	true	"VM UUID"
// @Param			id		path		string	true	"Container id or name"
// @Param			since	query		string	false	"Only lines written from this moment on (RFC 3339)"
// @Param			tail	query		int		false	"Only the last this many lines"
// @Success		200		{object}	getContainerLogs.Response
// @Failure		400		{object}	workload.Refusal
// @Failure		404		{object}	workload.Failure
// @Failure		500		{object}	workload.Failure
// @Router			/dashboard/workload/vms/{uuid}/containers/{id}/logs [get]
// @Router			/dashboard/my/workload/vms/{uuid}/containers/{id}/logs [get]
func (h *logsHandler) ServeHTTP(rw http.ResponseWriter, r *http.Request) {
	response, err := h.useCase.Execute(r.Context(), &getContainerLogs.Request{
		VMUUID:    r.PathValue("uuid"),
		ID:        r.PathValue("id"),
		Since:     workload.Since(r),
		Tail:      workload.Tail(r),
		OwnerUUID: h.owner(r),
	})

	switch {
	case workload.Failed(rw, r, err):
	case workload.Refused(rw, response.ValidationErrors):
	default:
		workload.JSON(rw, http.StatusOK, response)
	}
}
