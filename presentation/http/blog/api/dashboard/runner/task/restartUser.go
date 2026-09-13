package task

import (
	"errors"
	"net/http"

	"github.com/khanzadimahdi/testproject/application/auth"
	restartusertask "github.com/khanzadimahdi/testproject/application/dashboard/runner/task/restartUserTask"
	"github.com/khanzadimahdi/testproject/domain"
	infraTrace "github.com/khanzadimahdi/testproject/infrastructure/telemetry/trace"
	"go.opentelemetry.io/otel/trace"
)

type restartUserHandler struct {
	useCase *restartusertask.UseCase
}

func NewRestartUserHandler(useCase *restartusertask.UseCase) *restartUserHandler {
	return &restartUserHandler{useCase: useCase}
}

// @Summary		Restart own task
// @Description	restart one of your own tasks
// @Tags			dashboard runner
// @Param			uuid	path		string	true	"Task UUID"
// @Success		202		{object}	map[string]interface{}
// @Failure		404		{object}	map[string]interface{}
// @Router			/dashboard/my/runner/tasks/{uuid}/restart [post]
func (h *restartUserHandler) ServeHTTP(rw http.ResponseWriter, r *http.Request) {
	err := h.useCase.Execute(r.Context(), &restartusertask.Request{
		UUID:      r.PathValue("uuid"),
		OwnerUUID: auth.UUIDFromContext(r.Context()),
	})

	switch {
	case errors.Is(err, domain.ErrNotExists):
		rw.WriteHeader(http.StatusNotFound)
	case err != nil:
		infraTrace.RecordError(trace.SpanFromContext(r.Context()), err)
		rw.WriteHeader(http.StatusInternalServerError)
	default:
		rw.WriteHeader(http.StatusAccepted)
	}
}
