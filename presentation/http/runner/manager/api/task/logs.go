package task

import (
	"encoding/json"
	"net/http"
	"strconv"
	"time"

	gettasklogs "github.com/khanzadimahdi/testproject/application/runner/manager/task/getTaskLogs"
	infraTrace "github.com/khanzadimahdi/testproject/infrastructure/telemetry/trace"
	"go.opentelemetry.io/otel/trace"
)

type logsHandler struct {
	useCase *gettasklogs.UseCase
}

func NewLogsHandler(useCase *gettasklogs.UseCase) *logsHandler {
	return &logsHandler{
		useCase: useCase,
	}
}

// @Summary		Task logs
// @Description	read what a container has written, from its first line onward
// @Tags			runner tasks
// @Accept			json
// @Produce		json
// @Param			uuid	path		string	true	"Task UUID"
// @Param			after	query		string	false	"Only lines written after this moment (RFC3339)"
// @Param			limit	query		int		false	"How many lines to return"
// @Success		200		{object}	gettasklogs.Response
// @Failure		400		{object}	map[string]interface{}
// @Failure		500		{object}	map[string]interface{}
// @Router			/tasks/{uuid}/logs [get]
func (h *logsHandler) ServeHTTP(rw http.ResponseWriter, r *http.Request) {
	request := &gettasklogs.Request{
		UUID:  r.PathValue("uuid"),
		After: parseAfter(r),
	}

	if limit, err := strconv.ParseUint(r.URL.Query().Get("limit"), 10, 32); err == nil {
		request.Limit = uint(limit)
	}

	response, err := h.useCase.Execute(r.Context(), request)
	switch {
	case err != nil:
		infraTrace.RecordError(trace.SpanFromContext(r.Context()), err)
		rw.WriteHeader(http.StatusInternalServerError)
	case len(response.ValidationErrors) > 0:
		rw.Header().Add("Content-Type", "application/json")
		rw.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(rw).Encode(response)
	default:
		rw.Header().Add("Content-Type", "application/json")
		rw.WriteHeader(http.StatusOK)
		json.NewEncoder(rw).Encode(response)
	}
}

// parseAfter reads the moment a reader has already caught up to. An unreadable
// one is the same as none: the whole log from its beginning.
func parseAfter(r *http.Request) time.Time {
	after, err := time.Parse(time.RFC3339Nano, r.URL.Query().Get("after"))
	if err != nil {
		return time.Time{}
	}

	return after
}
