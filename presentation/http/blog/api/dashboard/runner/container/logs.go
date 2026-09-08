package container

import (
	"encoding/json"
	"net/http"
	"strconv"
	"time"

	getContainerLogs "github.com/khanzadimahdi/testproject/application/dashboard/runner/container/getContainerLogs"
	infraTrace "github.com/khanzadimahdi/testproject/infrastructure/telemetry/trace"
	"go.opentelemetry.io/otel/trace"
)

type logsHandler struct {
	useCase *getContainerLogs.UseCase
}

func NewLogsHandler(useCase *getContainerLogs.UseCase) *logsHandler {
	return &logsHandler{useCase: useCase}
}

// @Summary		Container logs
// @Description	read what a container has written, from its first line onward
// @Tags			dashboard runner
// @Accept			json
// @Produce		json
// @Param			uuid	path		string	true	"Container UUID"
// @Param			after	query		string	false	"Only lines written after this moment (RFC3339)"
// @Param			limit	query		int		false	"How many lines to return"
// @Success		200		{object}	getContainerLogs.Response
// @Failure		500		{object}	map[string]interface{}
// @Router			/dashboard/runner/containers/{uuid}/logs [get]
func (h *logsHandler) ServeHTTP(rw http.ResponseWriter, r *http.Request) {
	request := &getContainerLogs.Request{UUID: r.PathValue("uuid")}

	if after, err := time.Parse(time.RFC3339Nano, r.URL.Query().Get("after")); err == nil {
		request.After = after
	}

	if limit, err := strconv.ParseUint(r.URL.Query().Get("limit"), 10, 32); err == nil {
		request.Limit = uint(limit)
	}

	response, err := h.useCase.Execute(r.Context(), request)
	switch {
	case err != nil:
		infraTrace.RecordError(trace.SpanFromContext(r.Context()), err)
		rw.WriteHeader(http.StatusInternalServerError)
	default:
		rw.Header().Add("Content-Type", "application/json")
		rw.WriteHeader(http.StatusOK)
		json.NewEncoder(rw).Encode(response)
	}
}
