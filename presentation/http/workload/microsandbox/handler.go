// Package microsandbox serves the workload-microsandbox service's v1 API: the
// contract in infrastructure/workload/microsandbox/api, over the run
// supervisor.
//
// The handlers are thin. Each decodes what its route takes, asks the
// supervisor, and encodes what it answers; a failure is always the contract's
// ErrorResponse, its code the supervisor's own, so that what an orchestrator
// reads is what the supervisor meant. The server around them — mutual TLS, the
// listeners, the middleware — is the serve command's and the provider's.
package microsandbox

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"

	"go.opentelemetry.io/otel/trace"

	"github.com/khanzadimahdi/testproject/application/workload/microsandbox/runs"
	infraTrace "github.com/khanzadimahdi/testproject/infrastructure/telemetry/trace"
	"github.com/khanzadimahdi/testproject/infrastructure/workload/microsandbox/api"
)

// maxBody is the most a request's body may hold. A run's spec is the largest,
// and a megabyte is far more than any is.
const maxBody = 1 << 20

// NewHandler serves every route of the contract, and answers any other
// request with not_found, in the contract's own error.
func NewHandler(supervisor *runs.Supervisor, logger *slog.Logger) http.Handler {
	mux := http.NewServeMux()

	lifecycle := NewLifecycleHandlers(supervisor)

	mux.Handle(api.RouteInfo, NewInfoHandler(supervisor))
	mux.Handle(api.RouteListRuns, NewListRunsHandler(supervisor))
	mux.Handle(api.RouteCreateRun, NewCreateRunHandler(supervisor))
	mux.Handle(api.RouteGetRun, NewGetRunHandler(supervisor))
	mux.Handle(api.RouteDeleteRun, NewDeleteRunHandler(supervisor))
	mux.Handle(api.RouteStartRun, lifecycle.Start)
	mux.Handle(api.RouteStopRun, lifecycle.Stop)
	mux.Handle(api.RouteKillRun, lifecycle.Kill)
	mux.Handle(api.RouteRestartRun, lifecycle.Restart)
	mux.Handle(api.RouteRunLogs, NewLogsHandler(supervisor, logger))
	mux.Handle(api.RouteRunStats, NewRunStatsHandler(supervisor))
	mux.Handle(api.RouteExec, NewExecHandler(supervisor, logger))
	mux.Handle(api.RouteEndExec, NewEndExecHandler(supervisor))
	mux.Handle(api.RouteNodeStats, NewNodeStatsHandler(supervisor))
	mux.Handle(api.RoutePullImage, NewPullHandler(supervisor))

	mux.Handle("/", http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
		writeError(rw, r, &api.Error{
			Code:    api.CodeNotFound,
			Message: fmt.Sprintf("there is no route %s %s in v%s", r.Method, r.URL.Path, api.Version),
		})
	}))

	return mux
}

// statusOf is the HTTP status of a failure, by its code.
func statusOf(code string) int {
	switch code {
	case api.CodeInvalid, api.CodeNotSupported:
		return http.StatusBadRequest
	case api.CodeNotFound:
		return http.StatusNotFound
	case api.CodeNameInUse, api.CodeNotRunning, api.CodeCapacity:
		return http.StatusConflict
	case api.CodePullFailed:
		return http.StatusBadGateway
	case api.CodeUnavailable:
		return http.StatusServiceUnavailable
	default:
		return http.StatusInternalServerError
	}
}

// writeError answers with a failure, as the contract's ErrorResponse. One
// that is not the contract's own is internal, and its text is the message,
// which is what a task that could not be run is told.
func writeError(rw http.ResponseWriter, r *http.Request, err error) {
	var apiError *api.Error
	if !errors.As(err, &apiError) {
		apiError = &api.Error{Code: api.CodeInternal, Message: err.Error()}
	}

	status := statusOf(apiError.Code)

	if status >= http.StatusInternalServerError {
		infraTrace.RecordError(trace.SpanFromContext(r.Context()), err)
	}

	writeJSON(rw, status, api.ErrorResponse{Error: *apiError})
}

// writeJSON answers with a value, as JSON.
func writeJSON(rw http.ResponseWriter, status int, value any) {
	rw.Header().Set("Content-Type", "application/json")
	rw.WriteHeader(status)

	_ = json.NewEncoder(rw).Encode(value)
}

// decode reads a request's body into value. An empty body is an error unless
// the route takes none, when it leaves value as it was.
func decode(rw http.ResponseWriter, r *http.Request, value any, optional bool) error {
	err := json.NewDecoder(http.MaxBytesReader(rw, r.Body, maxBody)).Decode(value)

	switch {
	case err == nil:
		return nil
	case optional && errors.Is(err, io.EOF):
		return nil
	default:
		return &api.Error{Code: api.CodeInvalid, Message: fmt.Sprintf("the body could not be read: %v", err)}
	}
}
