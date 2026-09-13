package task

import (
	"encoding/json"
	"errors"
	"net/http"
	"strconv"

	getTasks "github.com/khanzadimahdi/testproject/application/dashboard/runner/task/getTasks"
	"github.com/khanzadimahdi/testproject/domain"
	infraTrace "github.com/khanzadimahdi/testproject/infrastructure/telemetry/trace"
	"go.opentelemetry.io/otel/trace"
)

type indexHandler struct {
	useCase *getTasks.UseCase
}

func NewIndexHandler(useCase *getTasks.UseCase) *indexHandler {
	return &indexHandler{useCase: useCase}
}

// @Summary		List tasks
// @Description	paginated list of the tasks the runner is holding
// @Tags			dashboard runner
// @Accept			json
// @Produce		json
// @Param			page	query		int	false	"Page"	default(1)
// @Success		200		{object}	getTasks.Response
// @Failure		500		{object}	map[string]interface{}
// @Router			/dashboard/runner/tasks [get]
func (h *indexHandler) ServeHTTP(rw http.ResponseWriter, r *http.Request) {
	var page uint = 1
	if parsed, err := strconv.ParseUint(r.URL.Query().Get("page"), 10, 32); err == nil {
		page = uint(parsed)
	}

	response, err := h.useCase.Execute(r.Context(), &getTasks.Request{Page: page})
	switch {
	case errors.Is(err, domain.ErrNotExists):
		rw.WriteHeader(http.StatusNotFound)
	case err != nil:
		infraTrace.RecordError(trace.SpanFromContext(r.Context()), err)
		rw.WriteHeader(http.StatusInternalServerError)
	default:
		rw.Header().Add("Content-Type", "application/json")
		rw.WriteHeader(http.StatusOK)
		json.NewEncoder(rw).Encode(response)
	}
}
