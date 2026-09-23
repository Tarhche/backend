// Package task serves the tasks this node is holding.
//
// The ingress cannot see a task: it works out which node has one and sends
// the request here. So this is the far end of that — the node reaching a
// task through whatever runs it, and saying so itself when it cannot.
package task

import (
	"context"
	"errors"
	"log/slog"
	"net"
	"net/http"
	"net/http/httputil"
	"net/url"
	"strconv"
	"time"

	getendpoint "github.com/khanzadimahdi/testproject/application/runner/orchestrator/task/getEndpoint"
	"github.com/khanzadimahdi/testproject/domain/runner/port"
	infraTrace "github.com/khanzadimahdi/testproject/infrastructure/telemetry/trace"
	"go.opentelemetry.io/otel/trace"
)

const (
	// idleConnectionTimeout is how long a connection to a task is kept for the
	// next request once the last one is done with it.
	idleConnectionTimeout = 90 * time.Second

	// maxIdleConnectionsPerTask caps how many of those are kept per task port.
	maxIdleConnectionsPerTask = 8
)

// Dialer connects to a port of one of the runs this node is holding. The
// runtime is one: how it gets there — a port docker published on its host, a
// stream into a microVM — is its own business.
type Dialer interface {
	Dial(ctx context.Context, executionID string, p port.Port) (net.Conn, error)
}

type proxyHandler struct {
	useCase *getendpoint.UseCase
	proxy   *httputil.ReverseProxy
	logger  *slog.Logger
}

var _ http.Handler = &proxyHandler{}

func NewProxyHandler(useCase *getendpoint.UseCase, dialer Dialer, logger *slog.Logger) *proxyHandler {
	h := &proxyHandler{useCase: useCase, logger: logger}

	h.proxy = &httputil.ReverseProxy{
		Rewrite: func(r *httputil.ProxyRequest) {
			upstream := r.In.Context().Value(targetKey{}).(*url.URL)

			r.SetURL(&url.URL{Scheme: upstream.Scheme, Host: upstream.Host})

			// set after SetURL: what the task is asked for is the path the
			// client asked the ingress for, not the route it arrived here on.
			r.Out.URL.Path = upstream.Path
			r.Out.URL.RawQuery = r.In.URL.RawQuery

			// the task is addressed by the name the client used, not by
			// whatever the runtime calls it.
			r.Out.Host = r.In.Host
		},
		// the address a request is sent to names a run and one of its ports
		// rather than a place on the network, so connecting to it is the
		// runtime's to do. Pooling still works as it would: connections are
		// kept per run and port.
		Transport: &http.Transport{
			DialContext: func(ctx context.Context, _ string, address string) (net.Conn, error) {
				executionID, p, err := splitAddress(address)
				if err != nil {
					return nil, err
				}

				return dialer.Dial(ctx, executionID, p)
			},
			IdleConnTimeout:     idleConnectionTimeout,
			MaxIdleConnsPerHost: maxIdleConnectionsPerTask,
		},
		ErrorHandler: func(rw http.ResponseWriter, _ *http.Request, err error) {
			// the task is there but not answering: its own problem to
			// report, not something the runner can fix.
			h.logger.Error("could not reach a task this node is holding", "error", err)
			http.Error(rw, "the task is not answering", http.StatusBadGateway)
		},
	}

	return h
}

// targetKey carries the resolved upstream from ServeHTTP to the rewrite, which
// is the only hook a ReverseProxy gives for a per-request target.
type targetKey struct{}

// @Summary		Serve a task
// @Description	carries the request to one of the tasks this node is holding, on one of the ports it exposes
// @Tags			runner tasks
// @Param			slug	path		string	true	"Task slug"
// @Param			port	path		int		true	"Task port, or 0 for the lowest it exposes"
// @Param			path	path		string	true	"Path on the task"
// @Success		200		{string}	string	"whatever the task answered"
// @Failure		404		{object}	map[string]interface{}
// @Failure		502		{object}	map[string]interface{}
// @Failure		503		{object}	map[string]interface{}
// @Router			/tasks/{slug}/{port}/{path} [get]
func (h *proxyHandler) ServeHTTP(rw http.ResponseWriter, r *http.Request) {
	requested, err := strconv.ParseUint(r.PathValue("port"), 10, 16)
	if err != nil {
		http.Error(rw, "that is not a port", http.StatusBadRequest)

		return
	}

	request := getendpoint.Request{
		Slug: r.PathValue("slug"),
		Port: port.Port(requested),
	}

	response, err := h.useCase.Execute(r.Context(), &request)

	switch {
	case errors.Is(err, getendpoint.ErrNotHeld):
		http.Error(rw, err.Error(), http.StatusNotFound)

		return
	case errors.Is(err, getendpoint.ErrNotExposed):
		http.Error(rw, err.Error(), http.StatusNotFound)

		return
	case errors.Is(err, getendpoint.ErrNotRunning):
		http.Error(rw, err.Error(), http.StatusServiceUnavailable)

		return
	case err != nil:
		infraTrace.RecordError(trace.SpanFromContext(r.Context()), err)
		rw.WriteHeader(http.StatusInternalServerError)

		return
	}

	target := &url.URL{
		Scheme: "http",
		Host:   net.JoinHostPort(response.ExecutionID, strconv.FormatUint(uint64(response.Port), 10)),
		Path:   "/" + r.PathValue("path"),
	}

	h.proxy.ServeHTTP(rw, r.WithContext(context.WithValue(r.Context(), targetKey{}, target)))
}

// splitAddress reads back the run and the port a request's address names.
func splitAddress(address string) (string, port.Port, error) {
	executionID, portNumber, err := net.SplitHostPort(address)
	if err != nil {
		return "", 0, err
	}

	p, err := strconv.ParseUint(portNumber, 10, 16)
	if err != nil {
		return "", 0, err
	}

	return executionID, port.Port(p), nil
}
