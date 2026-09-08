package container

import (
	"encoding/json"
	"errors"
	"net/http"

	"github.com/khanzadimahdi/testproject/application/auth"
	runContainer "github.com/khanzadimahdi/testproject/application/dashboard/runner/container/runContainer"
	"github.com/khanzadimahdi/testproject/domain"
	infraTrace "github.com/khanzadimahdi/testproject/infrastructure/telemetry/trace"
	"go.opentelemetry.io/otel/trace"
)

type runHandler struct {
	useCase *runContainer.UseCase
}

func NewRunHandler(useCase *runContainer.UseCase) *runHandler {
	return &runHandler{useCase: useCase}
}

// @Summary		Run a container
// @Description	run one long-running container from a docker compose service specification
// @Tags			dashboard runner
// @Accept			json
// @Produce		json
// @Param			body	body		runContainer.Request	true	"Container specification"
// @Success		201		{object}	runContainer.Response
// @Failure		400		{object}	map[string]interface{}
// @Failure		500		{object}	map[string]interface{}
// @Router			/dashboard/runner/containers [post]
func (h *runHandler) ServeHTTP(rw http.ResponseWriter, r *http.Request) {
	var request runContainer.Request
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
