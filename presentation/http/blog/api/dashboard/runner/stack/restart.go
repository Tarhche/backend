package stack

import (
	"errors"
	"net/http"

	restartStack "github.com/khanzadimahdi/testproject/application/dashboard/runner/stack/restartStack"
	"github.com/khanzadimahdi/testproject/domain"
	infraTrace "github.com/khanzadimahdi/testproject/infrastructure/telemetry/trace"
	"go.opentelemetry.io/otel/trace"
)

type restartHandler struct {
	useCase *restartStack.UseCase
}

func NewRestartHandler(useCase *restartStack.UseCase) *restartHandler {
	return &restartHandler{useCase: useCase}
}

// @Summary		Restart stack
// @Description	restart every service of a stack
// @Tags			dashboard runner
// @Param			uuid	path		string	true	"Stack UUID"
// @Success		202		{object}	map[string]interface{}
// @Failure		404		{object}	map[string]interface{}
// @Router			/dashboard/runner/stacks/{uuid}/restart [post]
func (h *restartHandler) ServeHTTP(rw http.ResponseWriter, r *http.Request) {
	err := h.useCase.Execute(r.Context(), &restartStack.Request{
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
