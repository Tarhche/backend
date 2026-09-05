package stack

import (
	"errors"
	"net/http"

	deleteStack "github.com/khanzadimahdi/testproject/application/dashboard/runner/stack/deleteStack"
	killStack "github.com/khanzadimahdi/testproject/application/dashboard/runner/stack/killStack"
	restartStack "github.com/khanzadimahdi/testproject/application/dashboard/runner/stack/restartStack"
	stopStack "github.com/khanzadimahdi/testproject/application/dashboard/runner/stack/stopStack"
	"github.com/khanzadimahdi/testproject/domain"
	infraTrace "github.com/khanzadimahdi/testproject/infrastructure/telemetry/trace"
	"go.opentelemetry.io/otel/trace"
)

// commandHandler carries one command to every service of a stack. The four
// differ only in which command they carry, so they share the handling of what
// can go wrong.
type commandHandler struct {
	command func(r *http.Request) error
	success int
}

var _ http.Handler = &commandHandler{}

func (h *commandHandler) ServeHTTP(rw http.ResponseWriter, r *http.Request) {
	err := h.command(r)

	switch {
	case errors.Is(err, domain.ErrNotExists):
		rw.WriteHeader(http.StatusNotFound)
	case err != nil:
		infraTrace.RecordError(trace.SpanFromContext(r.Context()), err)
		rw.WriteHeader(http.StatusInternalServerError)
	default:
		rw.WriteHeader(h.success)
	}
}

// @Summary		Stop stack
// @Description	stop every service of a stack, giving each a moment to shut down on its own
// @Tags			dashboard runner
// @Param			uuid	path		string	true	"Stack UUID"
// @Success		202		{object}	map[string]interface{}
// @Failure		404		{object}	map[string]interface{}
// @Router			/dashboard/runner/stacks/{uuid}/stop [post]
func NewStopHandler(useCase *stopStack.UseCase) http.Handler {
	return &commandHandler{
		success: http.StatusAccepted,
		command: func(r *http.Request) error {
			return useCase.Execute(r.Context(), &stopStack.Request{
				UUID: r.PathValue("uuid"),
			})
		},
	}
}

// @Summary		Kill stack
// @Description	stop every service of a stack at once, without a grace period
// @Tags			dashboard runner
// @Param			uuid	path		string	true	"Stack UUID"
// @Success		202		{object}	map[string]interface{}
// @Failure		404		{object}	map[string]interface{}
// @Router			/dashboard/runner/stacks/{uuid}/kill [post]
func NewKillHandler(useCase *killStack.UseCase) http.Handler {
	return &commandHandler{
		success: http.StatusAccepted,
		command: func(r *http.Request) error {
			return useCase.Execute(r.Context(), &killStack.Request{
				UUID: r.PathValue("uuid"),
			})
		},
	}
}

// @Summary		Restart stack
// @Description	restart every service of a stack
// @Tags			dashboard runner
// @Param			uuid	path		string	true	"Stack UUID"
// @Success		202		{object}	map[string]interface{}
// @Failure		404		{object}	map[string]interface{}
// @Router			/dashboard/runner/stacks/{uuid}/restart [post]
func NewRestartHandler(useCase *restartStack.UseCase) http.Handler {
	return &commandHandler{
		success: http.StatusAccepted,
		command: func(r *http.Request) error {
			return useCase.Execute(r.Context(), &restartStack.Request{
				UUID: r.PathValue("uuid"),
			})
		},
	}
}

// @Summary		Delete stack
// @Description	remove a stack and everything it holds: its ports, its log and the container itself
// @Tags			dashboard runner
// @Param			uuid	path		string	true	"Stack UUID"
// @Success		204		{object}	map[string]interface{}
// @Failure		404		{object}	map[string]interface{}
// @Router			/dashboard/runner/stacks/{uuid} [delete]
func NewDeleteHandler(useCase *deleteStack.UseCase) http.Handler {
	return &commandHandler{
		success: http.StatusNoContent,
		command: func(r *http.Request) error {
			return useCase.Execute(r.Context(), &deleteStack.Request{
				UUID: r.PathValue("uuid"),
			})
		},
	}
}
