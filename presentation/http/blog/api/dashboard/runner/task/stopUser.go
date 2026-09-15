package task

import (
	"errors"
	"net/http"

	"github.com/khanzadimahdi/testproject/application/auth"
	stopusertask "github.com/khanzadimahdi/testproject/application/dashboard/runner/task/stopUserTask"
	"github.com/khanzadimahdi/testproject/domain"
	infraTrace "github.com/khanzadimahdi/testproject/infrastructure/telemetry/trace"
	"go.opentelemetry.io/otel/trace"
)

type stopUserHandler struct {
	useCase *stopusertask.UseCase
}

func NewStopUserHandler(useCase *stopusertask.UseCase) *stopUserHandler {
	return &stopUserHandler{useCase: useCase}
}

// @Summary		Stop own task
// @Description	stop one of your own tasks
// @Tags			dashboard runner
// @Param			uuid	path		string	true	"Task UUID"
// @Success		202		{object}	map[string]interface{}
// @Failure		404		{object}	map[string]interface{}
// @Router			/dashboard/my/runner/tasks/{uuid}/stop [post]
func (h *stopUserHandler) ServeHTTP(rw http.ResponseWriter, r *http.Request) {
	err := h.useCase.Execute(r.Context(), &stopusertask.Request{
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
