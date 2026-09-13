package task

import (
	"encoding/json"
	"errors"
	"net/http"

	"github.com/khanzadimahdi/testproject/application/auth"
	getusertask "github.com/khanzadimahdi/testproject/application/dashboard/runner/task/getUserTask"
	"github.com/khanzadimahdi/testproject/domain"
	infraTrace "github.com/khanzadimahdi/testproject/infrastructure/telemetry/trace"
	"go.opentelemetry.io/otel/trace"
)

type showUserHandler struct {
	useCase *getusertask.UseCase
}

func NewShowUserHandler(useCase *getusertask.UseCase) *showUserHandler {
	return &showUserHandler{useCase: useCase}
}

// @Summary		Show own task
// @Description	one of the tasks the current user owns
// @Tags			dashboard runner
// @Accept			json
// @Produce		json
// @Param			uuid	path		string	true	"Task UUID"
// @Success		200		{object}	getusertask.Response
// @Failure		404		{object}	map[string]interface{}
// @Failure		500		{object}	map[string]interface{}
// @Router			/dashboard/my/runner/tasks/{uuid} [get]
func (h *showUserHandler) ServeHTTP(rw http.ResponseWriter, r *http.Request) {
	response, err := h.useCase.Execute(r.Context(), &getusertask.Request{
		UUID:      r.PathValue("uuid"),
		OwnerUUID: auth.UUIDFromContext(r.Context()),
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
