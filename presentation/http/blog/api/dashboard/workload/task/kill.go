package task

import (
	"errors"
	"net/http"

	killTask "github.com/khanzadimahdi/testproject/application/dashboard/workload/task/killTask"
	"github.com/khanzadimahdi/testproject/domain"
	infraTrace "github.com/khanzadimahdi/testproject/infrastructure/telemetry/trace"
	"go.opentelemetry.io/otel/trace"
)

type killHandler struct {
	useCase *killTask.UseCase
}

func NewKillHandler(useCase *killTask.UseCase) *killHandler {
	return &killHandler{useCase: useCase}
}

// @Summary		Kill task
// @Description	stop a task at once, without a grace period
// @Tags			dashboard workload
// @Param			uuid	path		string	true	"Task UUID"
// @Success		202		{object}	map[string]interface{}
// @Failure		404		{object}	map[string]interface{}
// @Router			/dashboard/workload/tasks/{uuid}/kill [post]
func (h *killHandler) ServeHTTP(rw http.ResponseWriter, r *http.Request) {
	err := h.useCase.Execute(r.Context(), &killTask.Request{
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
