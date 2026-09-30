package task

import (
	"encoding/json"
	"net/http"

	gettask "github.com/khanzadimahdi/testproject/application/runner/controlplane/task/getTask"
	runTask "github.com/khanzadimahdi/testproject/application/runner/controlplane/task/runTask"
	"github.com/khanzadimahdi/testproject/application/runner/spec"
	"github.com/khanzadimahdi/testproject/domain/runner/task"
	infraTrace "github.com/khanzadimahdi/testproject/infrastructure/telemetry/trace"
	"go.opentelemetry.io/otel/trace"
)

// taskRequest is one task to run, in the shape a compose service has.
type taskRequest struct {
	Name      string `json:"name"`
	OwnerUUID string `json:"owner_uuid"`

	Service spec.Service `json:"service"`
}

type runTaskHandler struct {
	runTask  *runTask.UseCase
	getTask  *gettask.UseCase
	defaults task.ResourceLimits
}

// NewRunTaskHandler runs a long-running task from a compose service.
func NewRunTaskHandler(runTaskUseCase *runTask.UseCase, getTaskUseCase *gettask.UseCase, defaults task.ResourceLimits) *runTaskHandler {
	return &runTaskHandler{
		runTask:  runTaskUseCase,
		getTask:  getTaskUseCase,
		defaults: defaults,
	}
}

// @Summary		Run a task
// @Description	run one long-running task from a docker compose service specification
// @Tags			runner tasks
// @Accept			json
// @Produce		json
// @Param			body	body		taskRequest	true	"Task specification"
// @Success		201		{object}	gettask.Response
// @Failure		400		{object}	map[string]interface{}
// @Failure		500		{object}	map[string]interface{}
// @Router			/tasks/run [post]
func (h *runTaskHandler) ServeHTTP(rw http.ResponseWriter, r *http.Request) {
	var request taskRequest
	if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
		rw.WriteHeader(http.StatusBadRequest)

		return
	}

	runRequest := runTask.FromSpec(request.Name, &request.Service, h.defaults)
	runRequest.OwnerUUID = request.OwnerUUID

	response, err := h.runTask.Execute(r.Context(), runRequest)
	switch {
	case err != nil:
		infraTrace.RecordError(trace.SpanFromContext(r.Context()), err)
		rw.WriteHeader(http.StatusInternalServerError)

		return
	case len(response.ValidationErrors) > 0:
		rw.Header().Add("Content-Type", "application/json")
		rw.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(rw).Encode(response)

		return
	}

	// answered with the task itself, so the caller has its slug and can
	// build the addresses its ports will be served on.
	created, err := h.getTask.Execute(r.Context(), response.UUID)
	if err != nil {
		infraTrace.RecordError(trace.SpanFromContext(r.Context()), err)
		rw.WriteHeader(http.StatusInternalServerError)

		return
	}

	rw.Header().Add("Content-Type", "application/json")
	rw.WriteHeader(http.StatusCreated)
	json.NewEncoder(rw).Encode(created)
}
