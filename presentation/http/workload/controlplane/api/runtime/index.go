// Package runtime serves the runtime classes the workload runs tasks with.
package runtime

import (
	"encoding/json"
	"net/http"

	"go.opentelemetry.io/otel/trace"

	getruntimes "github.com/khanzadimahdi/testproject/application/workload/controlplane/runtime/getRuntimes"
	infraTrace "github.com/khanzadimahdi/testproject/infrastructure/telemetry/trace"
)

type indexHandler struct {
	useCase *getruntimes.UseCase
}

func NewIndexHandler(useCase *getruntimes.UseCase) *indexHandler {
	return &indexHandler{useCase: useCase}
}

// @Summary		List runtime classes
// @Description	every class a task may be run with, in the order the platform names them: whether it is the default, how many nodes can run it right now, what all of those can do and what they hold between them
// @Tags			workload runtimes
// @Produce		json
// @Success		200	{object}	getruntimes.Response
// @Failure		500	{object}	map[string]interface{}
// @Router			/runtimes [get]
func (h *indexHandler) ServeHTTP(rw http.ResponseWriter, r *http.Request) {
	response, err := h.useCase.Execute(r.Context())
	if err != nil {
		infraTrace.RecordError(trace.SpanFromContext(r.Context()), err)
		rw.WriteHeader(http.StatusInternalServerError)

		return
	}

	rw.Header().Add("Content-Type", "application/json")
	rw.WriteHeader(http.StatusOK)
	json.NewEncoder(rw).Encode(response)
}
