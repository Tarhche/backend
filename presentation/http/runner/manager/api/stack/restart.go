package stack

import (
	"encoding/json"
	"errors"
	"net/http"

	restartstack "github.com/khanzadimahdi/testproject/application/runner/manager/stack/restartStack"
	"github.com/khanzadimahdi/testproject/domain"
	infraTrace "github.com/khanzadimahdi/testproject/infrastructure/telemetry/trace"
	"go.opentelemetry.io/otel/trace"
)

type restartHandler struct {
	useCase *restartstack.UseCase
}

func NewRestartHandler(useCase *restartstack.UseCase) *restartHandler {
	return &restartHandler{useCase: useCase}
}

// @Summary		Restart stack
// @Description	restart every service of a stack
// @Tags			runner stacks
// @Param			uuid	path		string	true	"Stack UUID"
// @Success		202		{object}	map[string]interface{}
// @Failure		404		{object}	map[string]interface{}
// @Router			/stacks/{uuid}/restart [post]
func (h *restartHandler) ServeHTTP(rw http.ResponseWriter, r *http.Request) {
	response, err := h.useCase.Execute(r.Context(), &restartstack.Request{UUID: r.PathValue("uuid")})

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
