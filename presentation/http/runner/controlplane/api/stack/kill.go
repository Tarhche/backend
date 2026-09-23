package stack

import (
	"encoding/json"
	"errors"
	"net/http"

	killstack "github.com/khanzadimahdi/testproject/application/runner/controlplane/stack/killStack"
	"github.com/khanzadimahdi/testproject/domain"
	infraTrace "github.com/khanzadimahdi/testproject/infrastructure/telemetry/trace"
	"go.opentelemetry.io/otel/trace"
)

type killHandler struct {
	useCase *killstack.UseCase
}

func NewKillHandler(useCase *killstack.UseCase) *killHandler {
	return &killHandler{useCase: useCase}
}

// @Summary		Kill stack
// @Description	stop every service of a stack at once, without a grace period
// @Tags			runner stacks
// @Param			uuid	path		string	true	"Stack UUID"
// @Success		202		{object}	map[string]interface{}
// @Failure		404		{object}	map[string]interface{}
// @Router			/stacks/{uuid}/kill [post]
func (h *killHandler) ServeHTTP(rw http.ResponseWriter, r *http.Request) {
	response, err := h.useCase.Execute(r.Context(), &killstack.Request{UUID: r.PathValue("uuid")})

	switch {
	case errors.Is(err, domain.ErrNotExists):
		rw.WriteHeader(http.StatusNotFound)
	case err != nil:
		infraTrace.RecordError(trace.SpanFromContext(r.Context()), err)
		rw.WriteHeader(http.StatusInternalServerError)
	case response != nil && len(response.ValidationErrors) > 0:
		rw.Header().Add("Content-Type", "application/json")
		rw.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(rw).Encode(response)
	default:
		rw.WriteHeader(http.StatusAccepted)
	}
}
