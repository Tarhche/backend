package stack

import (
	"encoding/json"
	"errors"
	"net/http"
	"strconv"

	getStacks "github.com/khanzadimahdi/testproject/application/dashboard/runner/stack/getStacks"
	"github.com/khanzadimahdi/testproject/domain"
	infraTrace "github.com/khanzadimahdi/testproject/infrastructure/telemetry/trace"
	"go.opentelemetry.io/otel/trace"
)

type indexHandler struct {
	useCase *getStacks.UseCase
}

func NewIndexHandler(useCase *getStacks.UseCase) *indexHandler {
	return &indexHandler{useCase: useCase}
}

// @Summary		List stacks
// @Description	paginated list of the stacks the runner is holding
// @Tags			dashboard runner
// @Accept			json
// @Produce		json
// @Param			page	query		int	false	"Page"	default(1)
// @Success		200		{object}	getStacks.Response
// @Failure		500		{object}	map[string]interface{}
// @Router			/dashboard/runner/stacks [get]
func (h *indexHandler) ServeHTTP(rw http.ResponseWriter, r *http.Request) {
	var page uint = 1
	if parsed, err := strconv.ParseUint(r.URL.Query().Get("page"), 10, 32); err == nil {
		page = uint(parsed)
	}

	response, err := h.useCase.Execute(r.Context(), &getStacks.Request{Page: page})
	switch {
	case errors.Is(err, domain.ErrForbidden):
		rw.WriteHeader(http.StatusForbidden)
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
