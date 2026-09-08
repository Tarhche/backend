package stack

import (
	"encoding/json"
	"net/http"
	"strconv"

	getstacks "github.com/khanzadimahdi/testproject/application/runner/manager/stack/getStacks"
	infraTrace "github.com/khanzadimahdi/testproject/infrastructure/telemetry/trace"
	"go.opentelemetry.io/otel/trace"
)

type indexHandler struct {
	useCase *getstacks.UseCase
}

func NewIndexHandler(useCase *getstacks.UseCase) *indexHandler {
	return &indexHandler{useCase: useCase}
}

// @Summary		List stacks
// @Description	return a page of stacks
// @Tags			runner stacks
// @Accept			json
// @Produce		json
// @Param			page	query		int	false	"Page number"	default(1)
// @Success		200		{object}	getstacks.Response
// @Failure		500		{object}	map[string]interface{}
// @Router			/stacks [get]
func (h *indexHandler) ServeHTTP(rw http.ResponseWriter, r *http.Request) {
	var page uint = 1
	if parsed, err := strconv.ParseUint(r.URL.Query().Get("page"), 10, 32); err == nil {
		page = uint(parsed)
	}

	response, err := h.useCase.Execute(r.Context(), &getstacks.Request{Page: page})
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
