package task

import (
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"time"

	"github.com/khanzadimahdi/testproject/application/auth"
	getusertasklogs "github.com/khanzadimahdi/testproject/application/dashboard/runner/task/getUserTaskLogs"
	"github.com/khanzadimahdi/testproject/domain"
	infraTrace "github.com/khanzadimahdi/testproject/infrastructure/telemetry/trace"
	"go.opentelemetry.io/otel/trace"
)

type logsUserHandler struct {
	useCase *getusertasklogs.UseCase
}

func NewLogsUserHandler(useCase *getusertasklogs.UseCase) *logsUserHandler {
	return &logsUserHandler{useCase: useCase}
}

// @Summary		Own task logs
// @Description	what one of your own tasks has written
// @Tags			dashboard runner
// @Accept			json
// @Produce		json
// @Param			uuid	path		string	true	"Task UUID"
// @Param			after	query		string	false	"Only lines written after this time (RFC3339)"
// @Param			limit	query		int		false	"How many lines"
// @Success		200		{object}	getusertasklogs.Response
// @Failure		404		{object}	map[string]interface{}
// @Failure		500		{object}	map[string]interface{}
// @Router			/dashboard/my/runner/tasks/{uuid}/logs [get]
func (h *logsUserHandler) ServeHTTP(rw http.ResponseWriter, r *http.Request) {
	request := &getusertasklogs.Request{
		UUID:      r.PathValue("uuid"),
		OwnerUUID: auth.UUIDFromContext(r.Context()),
	}

	if after, err := time.Parse(time.RFC3339Nano, r.URL.Query().Get("after")); err == nil {
		request.After = after
	}

	if limit, err := strconv.ParseUint(r.URL.Query().Get("limit"), 10, 32); err == nil {
		request.Limit = uint(limit)
	}

	response, err := h.useCase.Execute(r.Context(), request)
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
