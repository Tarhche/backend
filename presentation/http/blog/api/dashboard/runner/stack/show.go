package stack

import (
	"encoding/json"
	"errors"
	"net/http"

	getStack "github.com/khanzadimahdi/testproject/application/dashboard/runner/stack/getStack"
	"github.com/khanzadimahdi/testproject/domain"
	infraTrace "github.com/khanzadimahdi/testproject/infrastructure/telemetry/trace"
	"go.opentelemetry.io/otel/trace"
)

type showHandler struct {
	useCase *getStack.UseCase
}

func NewShowHandler(useCase *getStack.UseCase) *showHandler {
	return &showHandler{useCase: useCase}
}

// @Summary		Get stack
// @Description	retrieve a stack and the services in it
// @Tags			dashboard runner
// @Accept			json
// @Produce		json
// @Param			uuid	path		string	true	"Stack UUID"
// @Success		200		{object}	getStack.Response
// @Failure		404		{object}	map[string]interface{}
// @Failure		500		{object}	map[string]interface{}
// @Router			/dashboard/runner/stacks/{uuid} [get]
func (h *showHandler) ServeHTTP(rw http.ResponseWriter, r *http.Request) {
	response, err := h.useCase.Execute(r.Context(), r.PathValue("uuid"))
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
