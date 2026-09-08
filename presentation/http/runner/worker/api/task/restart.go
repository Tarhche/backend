package task

import (
	"encoding/json"
	"errors"
	"net/http"

	restarttask "github.com/khanzadimahdi/testproject/application/runner/worker/task/restartTask"
	"github.com/khanzadimahdi/testproject/domain"
	infraTrace "github.com/khanzadimahdi/testproject/infrastructure/telemetry/trace"
	"go.opentelemetry.io/otel/trace"
)

type restartHandler struct {
	useCase *restarttask.UseCase
}

func NewRestartHandler(useCase *restarttask.UseCase) *restartHandler {
	return &restartHandler{useCase: useCase}
}

// @Summary		Restart worker task
// @Description	stop a task container and start it again in place
// @Tags			runner tasks
// @Accept			json
// @Produce		json
// @Param			uuid	path		string	true	"Task UUID"
// @Success		202		{object}	map[string]interface{}
// @Failure		404		{object}	map[string]interface{}
// @Failure		500		{object}	map[string]interface{}
// @Router			/tasks/{uuid}/restart [post]
func (h *restartHandler) ServeHTTP(rw http.ResponseWriter, r *http.Request) {
	request := restarttask.Request{UUID: r.PathValue("uuid")}

	response, err := h.useCase.Execute(r.Context(), &request)
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
