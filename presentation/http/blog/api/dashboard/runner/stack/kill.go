package stack

import (
	"errors"
	"net/http"

	killStack "github.com/khanzadimahdi/testproject/application/dashboard/runner/stack/killStack"
	"github.com/khanzadimahdi/testproject/domain"
	infraTrace "github.com/khanzadimahdi/testproject/infrastructure/telemetry/trace"
	"go.opentelemetry.io/otel/trace"
)

type killHandler struct {
	useCase *killStack.UseCase
}

func NewKillHandler(useCase *killStack.UseCase) *killHandler {
	return &killHandler{useCase: useCase}
}

// @Summary		Kill stack
// @Description	stop every service of a stack at once, without a grace period
// @Tags			dashboard runner
// @Param			uuid	path		string	true	"Stack UUID"
// @Success		202		{object}	map[string]interface{}
// @Failure		404		{object}	map[string]interface{}
// @Router			/dashboard/runner/stacks/{uuid}/kill [post]
func (h *killHandler) ServeHTTP(rw http.ResponseWriter, r *http.Request) {
	err := h.useCase.Execute(r.Context(), &killStack.Request{
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
