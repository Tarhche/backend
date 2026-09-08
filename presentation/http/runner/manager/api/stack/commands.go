package stack

import (
	"encoding/json"
	"errors"
	"net/http"

	deletestack "github.com/khanzadimahdi/testproject/application/runner/manager/stack/deleteStack"
	killstack "github.com/khanzadimahdi/testproject/application/runner/manager/stack/killStack"
	restartstack "github.com/khanzadimahdi/testproject/application/runner/manager/stack/restartStack"
	stopstack "github.com/khanzadimahdi/testproject/application/runner/manager/stack/stopStack"
	"github.com/khanzadimahdi/testproject/domain"
	infraTrace "github.com/khanzadimahdi/testproject/infrastructure/telemetry/trace"
	"go.opentelemetry.io/otel/trace"
)

// refusal is what a stack command reports when the runner would not take it.
type refusal struct {
	ValidationErrors domain.ValidationErrors `json:"errors,omitempty"`
}

// commandHandler carries one stack-wide command. The four of them differ only
// in which command they carry and what they answer with, so they share a
// handler rather than four copies of the same error handling.
type commandHandler struct {
	command func(rw http.ResponseWriter, r *http.Request) (*refusal, error)
	success int
}

var _ http.Handler = &commandHandler{}

func (h *commandHandler) ServeHTTP(rw http.ResponseWriter, r *http.Request) {
	response, err := h.command(rw, r)

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
		rw.WriteHeader(h.success)
	}
}

// @Summary		Stop stack
// @Description	stop every service of a stack
// @Tags			runner stacks
// @Param			uuid	path		string	true	"Stack UUID"
// @Success		202		{object}	map[string]interface{}
// @Failure		404		{object}	map[string]interface{}
// @Router			/stacks/{uuid}/stop [post]
func NewStopHandler(useCase *stopstack.UseCase) http.Handler {
	return &commandHandler{
		success: http.StatusAccepted,
		command: func(_ http.ResponseWriter, r *http.Request) (*refusal, error) {
			response, err := useCase.Execute(r.Context(), &stopstack.Request{UUID: r.PathValue("uuid")})
			if response == nil {
				return nil, err
			}

			return &refusal{ValidationErrors: response.ValidationErrors}, err
		},
	}
}

// @Summary		Kill stack
// @Description	stop every service of a stack at once, without a grace period
// @Tags			runner stacks
// @Param			uuid	path		string	true	"Stack UUID"
// @Success		202		{object}	map[string]interface{}
// @Failure		404		{object}	map[string]interface{}
// @Router			/stacks/{uuid}/kill [post]
func NewKillHandler(useCase *killstack.UseCase) http.Handler {
	return &commandHandler{
		success: http.StatusAccepted,
		command: func(_ http.ResponseWriter, r *http.Request) (*refusal, error) {
			response, err := useCase.Execute(r.Context(), &killstack.Request{UUID: r.PathValue("uuid")})
			if response == nil {
				return nil, err
			}

			return &refusal{ValidationErrors: response.ValidationErrors}, err
		},
	}
}

// @Summary		Restart stack
// @Description	restart every service of a stack
// @Tags			runner stacks
// @Param			uuid	path		string	true	"Stack UUID"
// @Success		202		{object}	map[string]interface{}
// @Failure		404		{object}	map[string]interface{}
// @Router			/stacks/{uuid}/restart [post]
func NewRestartHandler(useCase *restartstack.UseCase) http.Handler {
	return &commandHandler{
		success: http.StatusAccepted,
		command: func(_ http.ResponseWriter, r *http.Request) (*refusal, error) {
			response, err := useCase.Execute(r.Context(), &restartstack.Request{UUID: r.PathValue("uuid")})
			if response == nil {
				return nil, err
			}

			return &refusal{ValidationErrors: response.ValidationErrors}, err
		},
	}
}

// @Summary		Delete stack
// @Description	remove a stack, its services, their logs and the network they shared
// @Tags			runner stacks
// @Param			uuid	path		string	true	"Stack UUID"
// @Success		204		{object}	map[string]interface{}
// @Failure		404		{object}	map[string]interface{}
// @Router			/stacks/{uuid} [delete]
func NewDeleteHandler(useCase *deletestack.UseCase) http.Handler {
	return &commandHandler{
		success: http.StatusNoContent,
		command: func(_ http.ResponseWriter, r *http.Request) (*refusal, error) {
			response, err := useCase.Execute(r.Context(), &deletestack.Request{UUID: r.PathValue("uuid")})
			if response == nil {
				return nil, err
			}

			return &refusal{ValidationErrors: response.ValidationErrors}, err
		},
	}
}
