// Package runtime serves the runtime classes a task may be run with, to the
// dashboard's forms and to the agents that run tasks through it.
package runtime

import (
	"encoding/json"
	"net/http"

	"go.opentelemetry.io/otel/trace"

	getRuntimes "github.com/khanzadimahdi/testproject/application/dashboard/workload/runtime/getRuntimes"
	infraTrace "github.com/khanzadimahdi/testproject/infrastructure/telemetry/trace"
)

type indexHandler struct {
	useCase *getRuntimes.UseCase
}

func NewIndexHandler(useCase *getRuntimes.UseCase) *indexHandler {
	return &indexHandler{useCase: useCase}
}

// @Summary		List runtime classes
// @Description	every class a task may be run with: whether it is the default, whether any node can run it right now, what the nodes that can are all able to do and what they hold between them
// @Tags			dashboard workload
// @Accept			json
// @Produce		json
// @Success		200	{object}	getRuntimes.Response
// @Failure		500	{object}	map[string]interface{}
// @Router			/dashboard/workload/runtimes [get]
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
