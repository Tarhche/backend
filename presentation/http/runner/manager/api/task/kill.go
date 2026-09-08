package task

import (
	"encoding/json"
	"errors"
	"net/http"

	killtask "github.com/khanzadimahdi/testproject/application/runner/manager/task/killTask"
	"github.com/khanzadimahdi/testproject/domain"
	infraTrace "github.com/khanzadimahdi/testproject/infrastructure/telemetry/trace"
	"go.opentelemetry.io/otel/trace"
)

type killHandler struct {
	useCase *killtask.UseCase
}

func NewKillHandler(useCase *killtask.UseCase) *killHandler {
	return &killHandler{
		useCase: useCase,
	}
}

// @Summary		Kill task
// @Description	stop a running task at once, without a grace period
// @Tags			runner tasks
// @Accept			json
// @Produce		json
// @Param			uuid	path		string	true	"Task UUID"
// @Success		202		{object}	map[string]interface{}
// @Failure		404		{object}	map[string]interface{}
// @Failure		500		{object}	map[string]interface{}
// @Router			/tasks/{uuid}/kill [post]
func (h *killHandler) ServeHTTP(rw http.ResponseWriter, r *http.Request) {
	request := &killtask.Request{
		UUID: r.PathValue("uuid"),
	}

	response, err := h.useCase.Execute(r.Context(), request)
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
