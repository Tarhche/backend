package stack

import (
	"encoding/json"
	"net/http"

	getstack "github.com/khanzadimahdi/testproject/application/runner/manager/stack/getStack"
	runstack "github.com/khanzadimahdi/testproject/application/runner/manager/stack/runStack"
	infraTrace "github.com/khanzadimahdi/testproject/infrastructure/telemetry/trace"
	"go.opentelemetry.io/otel/trace"
)

type runHandler struct {
	runStack *runstack.UseCase
	getStack *getstack.UseCase
}

func NewRunHandler(runStackUseCase *runstack.UseCase, getStackUseCase *getstack.UseCase) *runHandler {
	return &runHandler{
		runStack: runStackUseCase,
		getStack: getStackUseCase,
	}
}

// @Summary		Run a stack
// @Description	run a set of connected services from a docker compose specification
// @Tags			runner stacks
// @Accept			json
// @Produce		json
// @Param			body	body		runstack.Request	true	"Stack specification"
// @Success		201		{object}	getstack.Response
// @Failure		400		{object}	map[string]interface{}
// @Failure		500		{object}	map[string]interface{}
// @Router			/stacks/run [post]
func (h *runHandler) ServeHTTP(rw http.ResponseWriter, r *http.Request) {
	var request runstack.Request
	if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
		rw.WriteHeader(http.StatusBadRequest)

		return
	}

	response, err := h.runStack.Execute(r.Context(), &request)
	switch {
	case err != nil:
		infraTrace.RecordError(trace.SpanFromContext(r.Context()), err)
		rw.WriteHeader(http.StatusInternalServerError)

		return
	case len(response.ValidationErrors) > 0:
		rw.Header().Add("Content-Type", "application/json")
		rw.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(rw).Encode(response)

		return
	}

	created, err := h.getStack.Execute(r.Context(), response.UUID)
	if err != nil {
		infraTrace.RecordError(trace.SpanFromContext(r.Context()), err)
		rw.WriteHeader(http.StatusInternalServerError)

		return
	}

	rw.Header().Add("Content-Type", "application/json")
	rw.WriteHeader(http.StatusCreated)
	json.NewEncoder(rw).Encode(created)
}
