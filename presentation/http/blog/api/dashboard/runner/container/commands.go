package container

import (
	"errors"
	"net/http"

	deleteContainer "github.com/khanzadimahdi/testproject/application/dashboard/runner/container/deleteContainer"
	killContainer "github.com/khanzadimahdi/testproject/application/dashboard/runner/container/killContainer"
	restartContainer "github.com/khanzadimahdi/testproject/application/dashboard/runner/container/restartContainer"
	stopContainer "github.com/khanzadimahdi/testproject/application/dashboard/runner/container/stopContainer"
	"github.com/khanzadimahdi/testproject/domain"
	infraTrace "github.com/khanzadimahdi/testproject/infrastructure/telemetry/trace"
	"go.opentelemetry.io/otel/trace"
)

// commandHandler carries one command to a container. The four differ only in
// which command they carry, so they share the handling of what can go wrong.
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

// @Summary		Stop container
// @Description	stop a container, giving it a moment to shut down on its own
// @Tags			dashboard runner
// @Param			uuid	path		string	true	"Container UUID"
// @Success		202		{object}	map[string]interface{}
// @Failure		404		{object}	map[string]interface{}
// @Router			/dashboard/runner/containers/{uuid}/stop [post]
func NewStopHandler(useCase *stopContainer.UseCase) http.Handler {
	return &commandHandler{
		success: http.StatusAccepted,
		command: func(r *http.Request) error {
			return useCase.Execute(r.Context(), &stopContainer.Request{
				UUID: r.PathValue("uuid"),
			})
		},
	}
}

// @Summary		Kill container
// @Description	stop a container at once, without a grace period
// @Tags			dashboard runner
// @Param			uuid	path		string	true	"Container UUID"
// @Success		202		{object}	map[string]interface{}
// @Failure		404		{object}	map[string]interface{}
// @Router			/dashboard/runner/containers/{uuid}/kill [post]
func NewKillHandler(useCase *killContainer.UseCase) http.Handler {
	return &commandHandler{
		success: http.StatusAccepted,
		command: func(r *http.Request) error {
			return useCase.Execute(r.Context(), &killContainer.Request{
				UUID: r.PathValue("uuid"),
			})
		},
	}
}

// @Summary		Restart container
// @Description	stop a container and start it again in place
// @Tags			dashboard runner
// @Param			uuid	path		string	true	"Container UUID"
// @Success		202		{object}	map[string]interface{}
// @Failure		404		{object}	map[string]interface{}
// @Router			/dashboard/runner/containers/{uuid}/restart [post]
func NewRestartHandler(useCase *restartContainer.UseCase) http.Handler {
	return &commandHandler{
		success: http.StatusAccepted,
		command: func(r *http.Request) error {
			return useCase.Execute(r.Context(), &restartContainer.Request{
				UUID: r.PathValue("uuid"),
			})
		},
	}
}

// @Summary		Delete container
// @Description	remove a container and everything it holds: its ports, its log and the container itself
// @Tags			dashboard runner
// @Param			uuid	path		string	true	"Container UUID"
// @Success		204		{object}	map[string]interface{}
// @Failure		404		{object}	map[string]interface{}
// @Router			/dashboard/runner/containers/{uuid} [delete]
func NewDeleteHandler(useCase *deleteContainer.UseCase) http.Handler {
	return &commandHandler{
		success: http.StatusNoContent,
		command: func(r *http.Request) error {
			return useCase.Execute(r.Context(), &deleteContainer.Request{
				UUID: r.PathValue("uuid"),
			})
		},
	}
}
