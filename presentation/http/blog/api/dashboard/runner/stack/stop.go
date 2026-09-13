package stack

import (
	"errors"
	"net/http"

	stopStack "github.com/khanzadimahdi/testproject/application/dashboard/runner/stack/stopStack"
	"github.com/khanzadimahdi/testproject/domain"
	infraTrace "github.com/khanzadimahdi/testproject/infrastructure/telemetry/trace"
	"go.opentelemetry.io/otel/trace"
)

type stopHandler struct {
	useCase *stopStack.UseCase
}

func NewStopHandler(useCase *stopStack.UseCase) *stopHandler {
	return &stopHandler{useCase: useCase}
}

// @Summary		Stop stack
// @Description	stop every service of a stack, giving each a moment to shut down on its own
// @Tags			dashboard runner
// @Param			uuid	path		string	true	"Stack UUID"
// @Success		202		{object}	map[string]interface{}
// @Failure		404		{object}	map[string]interface{}
// @Router			/dashboard/runner/stacks/{uuid}/stop [post]
func (h *stopHandler) ServeHTTP(rw http.ResponseWriter, r *http.Request) {
	err := h.useCase.Execute(r.Context(), &stopStack.Request{
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
