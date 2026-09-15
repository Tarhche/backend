package task

import (
	"errors"
	"net/http"

	stopTask "github.com/khanzadimahdi/testproject/application/dashboard/runner/task/stopTask"
	"github.com/khanzadimahdi/testproject/domain"
	infraTrace "github.com/khanzadimahdi/testproject/infrastructure/telemetry/trace"
	"go.opentelemetry.io/otel/trace"
)

type stopHandler struct {
	useCase *stopTask.UseCase
}

func NewStopHandler(useCase *stopTask.UseCase) *stopHandler {
	return &stopHandler{useCase: useCase}
}

// @Summary		Stop task
// @Description	stop a task, giving it a moment to shut down on its own
// @Tags			dashboard runner
// @Param			uuid	path		string	true	"Task UUID"
// @Success		202		{object}	map[string]interface{}
// @Failure		404		{object}	map[string]interface{}
// @Router			/dashboard/runner/tasks/{uuid}/stop [post]
func (h *stopHandler) ServeHTTP(rw http.ResponseWriter, r *http.Request) {
	err := h.useCase.Execute(r.Context(), &stopTask.Request{
		UUID: r.PathValue("uuid"),
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
