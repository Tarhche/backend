package task

import (
	"encoding/json"
	"errors"
	"net/http"

	getTask "github.com/khanzadimahdi/testproject/application/dashboard/runner/task/getTask"
	"github.com/khanzadimahdi/testproject/domain"
	infraTrace "github.com/khanzadimahdi/testproject/infrastructure/telemetry/trace"
	"go.opentelemetry.io/otel/trace"
)

type showHandler struct {
	useCase *getTask.UseCase
}

func NewShowHandler(useCase *getTask.UseCase) *showHandler {
	return &showHandler{useCase: useCase}
}

// @Summary		Get task
// @Description	retrieve one task, with the addresses its ports are served on
// @Tags			dashboard runner
// @Accept			json
// @Produce		json
// @Param			uuid	path		string	true	"Task UUID"
// @Success		200		{object}	getTask.Response
// @Failure		404		{object}	map[string]interface{}
// @Failure		500		{object}	map[string]interface{}
// @Router			/dashboard/runner/tasks/{uuid} [get]
func (h *showHandler) ServeHTTP(rw http.ResponseWriter, r *http.Request) {
	response, err := h.useCase.Execute(r.Context(), &getTask.Request{
		UUID: r.PathValue("uuid"),
	})
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
