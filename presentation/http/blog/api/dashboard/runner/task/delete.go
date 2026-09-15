package task

import (
	"errors"
	"net/http"

	deleteTask "github.com/khanzadimahdi/testproject/application/dashboard/runner/task/deleteTask"
	"github.com/khanzadimahdi/testproject/domain"
	infraTrace "github.com/khanzadimahdi/testproject/infrastructure/telemetry/trace"
	"go.opentelemetry.io/otel/trace"
)

type deleteHandler struct {
	useCase *deleteTask.UseCase
}

func NewDeleteHandler(useCase *deleteTask.UseCase) *deleteHandler {
	return &deleteHandler{useCase: useCase}
}

// @Summary		Delete task
// @Description	remove a task and everything it holds: its ports, its log and the task itself
// @Tags			dashboard runner
// @Param			uuid	path		string	true	"Task UUID"
// @Success		204		{object}	map[string]interface{}
// @Failure		404		{object}	map[string]interface{}
// @Router			/dashboard/runner/tasks/{uuid} [delete]
func (h *deleteHandler) ServeHTTP(rw http.ResponseWriter, r *http.Request) {
	err := h.useCase.Execute(r.Context(), &deleteTask.Request{
		UUID: r.PathValue("uuid"),
	})

	switch {
	case errors.Is(err, domain.ErrNotExists):
		rw.WriteHeader(http.StatusNotFound)
	case err != nil:
		infraTrace.RecordError(trace.SpanFromContext(r.Context()), err)
		rw.WriteHeader(http.StatusInternalServerError)
	default:
		rw.WriteHeader(http.StatusNoContent)
	}
}
