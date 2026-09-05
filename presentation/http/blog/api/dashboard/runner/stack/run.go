package stack

import (
	"encoding/json"
	"errors"
	"net/http"

	"github.com/khanzadimahdi/testproject/application/auth"
	runStack "github.com/khanzadimahdi/testproject/application/dashboard/runner/stack/runStack"
	"github.com/khanzadimahdi/testproject/domain"
	infraTrace "github.com/khanzadimahdi/testproject/infrastructure/telemetry/trace"
	"go.opentelemetry.io/otel/trace"
)

type runHandler struct {
	useCase *runStack.UseCase
}

func NewRunHandler(useCase *runStack.UseCase) *runHandler {
	return &runHandler{useCase: useCase}
}

// @Summary		Run a stack
// @Description	run a set of connected services from a docker compose specification
// @Tags			dashboard runner
// @Accept			json
// @Produce		json
// @Param			body	body		runStack.Request	true	"Stack specification"
// @Success		201		{object}	runStack.Response
// @Failure		400		{object}	map[string]interface{}
// @Failure		500		{object}	map[string]interface{}
// @Router			/dashboard/runner/stacks [post]
func (h *runHandler) ServeHTTP(rw http.ResponseWriter, r *http.Request) {
	var request runStack.Request
	if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
		rw.Header().Add("Content-Type", "application/json")
		rw.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(rw).Encode(map[string]any{"errors": map[string]string{"body": "invalid_value"}})

		return
	}

	request.OwnerUUID = auth.UUIDFromContext(r.Context())

	response, err := h.useCase.Execute(r.Context(), &request)
	switch {
	case errors.Is(err, domain.ErrNotExists):
		rw.WriteHeader(http.StatusNotFound)
	case err != nil:
		infraTrace.RecordError(trace.SpanFromContext(r.Context()), err)
		rw.WriteHeader(http.StatusInternalServerError)
	case len(response.ValidationErrors) > 0:
		rw.Header().Add("Content-Type", "application/json")
		rw.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(rw).Encode(response)
	default:
		rw.Header().Add("Content-Type", "application/json")
		rw.WriteHeader(http.StatusCreated)
		json.NewEncoder(rw).Encode(response)
	}
}
