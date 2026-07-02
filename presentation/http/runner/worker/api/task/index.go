package task

import (
	"encoding/json"
	"net/http"

	gettasks "github.com/khanzadimahdi/testproject/application/runner/worker/task/getTasks"
	infraTrace "github.com/khanzadimahdi/testproject/infrastructure/telemetry/trace"
	"go.opentelemetry.io/otel/trace"
)

type indexHandler struct {
	useCase *gettasks.UseCase
}

func NewIndexHandler(useCase *gettasks.UseCase) *indexHandler {
	return &indexHandler{
		useCase: useCase,
	}
}

// @Summary		List worker tasks
// @Description	return tasks scheduled for this worker
// @Tags			runner tasks
// @Accept			json
// @Produce		json
// @Success		200	{object}	gettasks.Response
// @Failure		500	{object}	map[string]interface{}
// @Router			/tasks [get]
func (h *indexHandler) ServeHTTP(rw http.ResponseWriter, r *http.Request) {
	response, err := h.useCase.Execute(r.Context())
	switch {
	case err != nil:
		infraTrace.RecordError(trace.SpanFromContext(r.Context()), err)
		rw.WriteHeader(http.StatusInternalServerError)
	default:
		rw.Header().Add("Content-Type", "application/json")
		rw.WriteHeader(http.StatusOK)
		json.NewEncoder(rw).Encode(response)
	}
}
