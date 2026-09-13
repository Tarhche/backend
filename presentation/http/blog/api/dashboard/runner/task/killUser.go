package task

import (
	"errors"
	"net/http"

	"github.com/khanzadimahdi/testproject/application/auth"
	killusertask "github.com/khanzadimahdi/testproject/application/dashboard/runner/task/killUserTask"
	"github.com/khanzadimahdi/testproject/domain"
	infraTrace "github.com/khanzadimahdi/testproject/infrastructure/telemetry/trace"
	"go.opentelemetry.io/otel/trace"
)

type killUserHandler struct {
	useCase *killusertask.UseCase
}

func NewKillUserHandler(useCase *killusertask.UseCase) *killUserHandler {
	return &killUserHandler{useCase: useCase}
}

// @Summary		Kill own task
// @Description	stop one of your own tasks at once
// @Tags			dashboard runner
// @Param			uuid	path		string	true	"Task UUID"
// @Success		202		{object}	map[string]interface{}
// @Failure		404		{object}	map[string]interface{}
// @Router			/dashboard/my/runner/tasks/{uuid}/kill [post]
func (h *killUserHandler) ServeHTTP(rw http.ResponseWriter, r *http.Request) {
	err := h.useCase.Execute(r.Context(), &killusertask.Request{
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
		rw.WriteHeader(http.StatusAccepted)
	}
}
