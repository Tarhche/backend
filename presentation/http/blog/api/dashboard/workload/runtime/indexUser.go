package runtime

import (
	"encoding/json"
	"net/http"

	"go.opentelemetry.io/otel/trace"

	getUserRuntimes "github.com/khanzadimahdi/testproject/application/dashboard/workload/runtime/getUserRuntimes"
	infraTrace "github.com/khanzadimahdi/testproject/infrastructure/telemetry/trace"
)

// indexUserHandler lists the classes the person asking may run their own
// tasks with. It answers what the listing of everybody's answers, behind the
// permission somebody running only their own tasks holds.
type indexUserHandler struct {
	useCase *getUserRuntimes.UseCase
}

func NewIndexUserHandler(useCase *getUserRuntimes.UseCase) *indexUserHandler {
	return &indexUserHandler{useCase: useCase}
}

// @Summary		List the runtime classes of my tasks
// @Description	every class the current user's own tasks may be run with: whether it is the default, whether any node can run it right now, what the nodes that can are all able to do and what they hold between them
// @Tags			dashboard workload
// @Accept			json
// @Produce		json
// @Success		200	{object}	getUserRuntimes.Response
// @Failure		500	{object}	map[string]interface{}
// @Router			/dashboard/my/workload/runtimes [get]
func (h *indexUserHandler) ServeHTTP(rw http.ResponseWriter, r *http.Request) {
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
