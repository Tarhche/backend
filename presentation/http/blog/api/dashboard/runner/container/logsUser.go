package container

import (
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"time"

	"github.com/khanzadimahdi/testproject/application/auth"
	getusercontainerlogs "github.com/khanzadimahdi/testproject/application/dashboard/runner/container/getUserContainerLogs"
	"github.com/khanzadimahdi/testproject/domain"
	infraTrace "github.com/khanzadimahdi/testproject/infrastructure/telemetry/trace"
	"go.opentelemetry.io/otel/trace"
)

type logsUserHandler struct {
	useCase *getusercontainerlogs.UseCase
}

func NewLogsUserHandler(useCase *getusercontainerlogs.UseCase) *logsUserHandler {
	return &logsUserHandler{useCase: useCase}
}

// @Summary		Own container logs
// @Description	what one of your own containers has written
// @Tags			dashboard runner
// @Accept			json
// @Produce		json
// @Param			uuid	path		string	true	"Container UUID"
// @Param			after	query		string	false	"Only lines written after this time (RFC3339)"
// @Param			limit	query		int		false	"How many lines"
// @Success		200		{object}	getusercontainerlogs.Response
// @Failure		404		{object}	map[string]interface{}
// @Failure		500		{object}	map[string]interface{}
// @Router			/dashboard/my/runner/containers/{uuid}/logs [get]
func (h *logsUserHandler) ServeHTTP(rw http.ResponseWriter, r *http.Request) {
	request := &getusercontainerlogs.Request{
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
