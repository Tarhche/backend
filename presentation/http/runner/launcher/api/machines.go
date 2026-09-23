// Package api serves the launcher's orders: machines to start and to end, and
// the networks they are plugged into.
//
// It is reached through a unix socket on the host and by nothing else, and
// the only thing that reaches it is an orchestrator. So an answer that is a
// failure says why, which an answer on a public route would not: why a machine
// did not start is exactly what the orchestrator has to report.
package api

import (
	"encoding/json"
	"net/http"

	"go.opentelemetry.io/otel/trace"

	"github.com/khanzadimahdi/testproject/application/runner/launcher/getMachines"
	"github.com/khanzadimahdi/testproject/application/runner/launcher/launchMachine"
	"github.com/khanzadimahdi/testproject/application/runner/launcher/terminateMachine"
	infraTrace "github.com/khanzadimahdi/testproject/infrastructure/telemetry/trace"
)

type launchHandler struct {
	useCase *launchMachine.UseCase
}

func NewLaunchHandler(useCase *launchMachine.UseCase) *launchHandler {
	return &launchHandler{useCase: useCase}
}

// @Summary		Launch a machine
// @Description	starts a machine's firecracker and plugs it into its networks
// @Tags			runner launcher
// @Accept			json
// @Produce		json
// @Param			body	body		launchMachine.Request	true	"Machine"
// @Success		201		{object}	launchMachine.Response
// @Failure		400		{object}	launchMachine.Response
// @Failure		500		{string}	string	"why it could not be started"
// @Router			/machines [post]
func (h *launchHandler) ServeHTTP(rw http.ResponseWriter, r *http.Request) {
	var request launchMachine.Request
	if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
		rw.WriteHeader(http.StatusBadRequest)

		return
	}

	response, err := h.useCase.Execute(r.Context(), &request)

	switch {
	case err != nil:
		failed(rw, r, err)
	case len(response.ValidationErrors) > 0:
		reply(rw, http.StatusBadRequest, response)
	default:
		reply(rw, http.StatusCreated, response)
	}
}

type indexHandler struct {
	useCase *getMachines.UseCase
}

func NewIndexHandler(useCase *getMachines.UseCase) *indexHandler {
	return &indexHandler{useCase: useCase}
}

// @Summary		Machines
// @Description	every machine this host holds for an orchestrator, running or not
// @Tags			runner launcher
// @Produce		json
// @Param			owner	query		string	true	"Orchestrator"
// @Success		200		{object}	getMachines.Response
// @Failure		400		{object}	getMachines.Response
// @Failure		500		{string}	string	"why they could not be listed"
// @Router			/machines [get]
func (h *indexHandler) ServeHTTP(rw http.ResponseWriter, r *http.Request) {
	request := getMachines.Request{Owner: r.URL.Query().Get("owner")}

	response, err := h.useCase.Execute(r.Context(), &request)

	switch {
	case err != nil:
		failed(rw, r, err)
	case len(response.ValidationErrors) > 0:
		reply(rw, http.StatusBadRequest, response)
	default:
		reply(rw, http.StatusOK, response)
	}
}

type terminateHandler struct {
	useCase *terminateMachine.UseCase
}

func NewTerminateHandler(useCase *terminateMachine.UseCase) *terminateHandler {
	return &terminateHandler{useCase: useCase}
}

// @Summary		Terminate a machine
// @Description	ends a machine's firecracker, unplugs it, and lets go of what the host kept for it
// @Tags			runner launcher
// @Param			id	path	string	true	"Machine"
// @Success		204
// @Failure		400	{object}	terminateMachine.Response
// @Failure		500	{string}	string	"why it could not be terminated"
// @Router			/machines/{id} [delete]
func (h *terminateHandler) ServeHTTP(rw http.ResponseWriter, r *http.Request) {
	request := terminateMachine.Request{ID: r.PathValue("id")}

	response, err := h.useCase.Execute(r.Context(), &request)

	switch {
	case err != nil:
		failed(rw, r, err)
	case len(response.ValidationErrors) > 0:
		reply(rw, http.StatusBadRequest, response)
	default:
		rw.WriteHeader(http.StatusNoContent)
	}
}

func reply(rw http.ResponseWriter, status int, response any) {
	rw.Header().Add("Content-Type", "application/json")
	rw.WriteHeader(status)
	_ = json.NewEncoder(rw).Encode(response)
}

// failed answers a failure, and says why.
func failed(rw http.ResponseWriter, r *http.Request, err error) {
	infraTrace.RecordError(trace.SpanFromContext(r.Context()), err)
	http.Error(rw, err.Error(), http.StatusInternalServerError)
}
