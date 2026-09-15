package task

import (
	"errors"
	"net/http"

	restartTask "github.com/khanzadimahdi/testproject/application/dashboard/runner/task/restartTask"
	"github.com/khanzadimahdi/testproject/domain"
	infraTrace "github.com/khanzadimahdi/testproject/infrastructure/telemetry/trace"
	"go.opentelemetry.io/otel/trace"
)

type restartHandler struct {
	useCase *restartTask.UseCase
}

func NewRestartHandler(useCase *restartTask.UseCase) *restartHandler {
	return &restartHandler{useCase: useCase}
}

// @Summary		Restart task
// @Description	stop a task and start it again in place
// @Tags			dashboard runner
// @Param			uuid	path		string	true	"Task UUID"
// @Success		202		{object}	map[string]interface{}
// @Failure		404		{object}	map[string]interface{}
// @Router			/dashboard/runner/tasks/{uuid}/restart [post]
func (h *restartHandler) ServeHTTP(rw http.ResponseWriter, r *http.Request) {
	err := h.useCase.Execute(r.Context(), &restartTask.Request{
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
