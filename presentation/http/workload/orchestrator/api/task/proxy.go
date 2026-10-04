// Package task serves the tasks this node is holding.
//
// The ingress cannot see a task: it works out which node has one and sends
// the request here. So this is the far end of that — the node reaching a
// task through whatever runs it, and saying so itself when it cannot.
package task

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"net/http/httputil"
	"net/url"
	"strconv"
	"time"

	getendpoint "github.com/khanzadimahdi/testproject/application/workload/orchestrator/task/getEndpoint"
	"github.com/khanzadimahdi/testproject/domain/workload/port"
	infraTrace "github.com/khanzadimahdi/testproject/infrastructure/telemetry/trace"
	"go.opentelemetry.io/otel/trace"
)

const (
	// idleConnectionTimeout is how long a connection to a task is kept for the
	// next request once the last one is done with it.
	idleConnectionTimeout = 90 * time.Second

	// maxIdleConnectionsPerTask caps how many of those are kept per task port.
	maxIdleConnectionsPerTask = 8

	// dialTimeout bounds connecting to a task. A connection is made for a
	// request but may serve another, so it is not cut short with the request
	// that asked for it, and is bounded on its own instead.
	dialTimeout = 10 * time.Second
)

type proxyHandler struct {
	useCase *getendpoint.UseCase
	proxy   *httputil.ReverseProxy
	logger  *slog.Logger
}

var _ http.Handler = &proxyHandler{}

func NewProxyHandler(useCase *getendpoint.UseCase, logger *slog.Logger) *proxyHandler {
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
			// wherever this node reaches it.
			r.Out.Host = r.In.Host
		},
		// the address a request is sent to names one of a run's ports rather
		// than a place on the network, so connecting to it is the use case's
		// to do: through the runtime, when the runtime reaches its runs
		// itself, or at the port docker published. The stream that comes back
		// is plain TCP either way, so a websocket upgrade travels down it as
		// it would down any connection. Connections are still pooled, by run
		// and port.
		Transport: &http.Transport{
			DialContext: func(ctx context.Context, _ string, address string) (net.Conn, error) {
				endpoint, err := endpointOf(address)
				if err != nil {
					return nil, err
				}

				ctx, cancel := context.WithTimeout(ctx, dialTimeout)
				defer cancel()

				return h.useCase.Dial(ctx, endpoint)
			},
			IdleConnTimeout:     idleConnectionTimeout,
			MaxIdleConnsPerHost: maxIdleConnectionsPerTask,
		},
		ErrorHandler: func(rw http.ResponseWriter, _ *http.Request, err error) {
			// the task is there but not answering: its own problem to
			// report, not something the workload can fix.
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
// @Description	carries the request to one of the tasks this node is holding, on one of the ports it exposes, through whatever runs it
// @Tags			workload tasks
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

	address, err := addressOf(response)
	if err != nil {
		infraTrace.RecordError(trace.SpanFromContext(r.Context()), err)
		rw.WriteHeader(http.StatusInternalServerError)

		return
	}

	target := &url.URL{
		Scheme: "http",
		Host:   address,
		Path:   "/" + r.PathValue("path"),
	}

	h.proxy.ServeHTTP(rw, r.WithContext(context.WithValue(r.Context(), targetKey{}, target)))
}

// addressOf writes where a request goes as the address the proxy's transport
// is handed, which endpointOf reads back when the transport dials it.
//
// The transport keeps connections by address, so the address names everything
// about where a connection leads — the run, its port, and where that port was
// published — and a connection kept for one is never handed to a request for
// another. It is written in hex, so that nothing in it, such as the colon an
// execution ID carries its class behind, can be read as part of the address.
func addressOf(endpoint *getendpoint.Response) (string, error) {
	encoded, err := json.Marshal(endpoint)
	if err != nil {
		return "", err
	}

	return net.JoinHostPort(hex.EncodeToString(encoded), strconv.FormatUint(uint64(endpoint.Port), 10)), nil
}

// endpointOf reads back where a request goes from the address addressOf wrote.
func endpointOf(address string) (*getendpoint.Response, error) {
	host, _, err := net.SplitHostPort(address)
	if err != nil {
		return nil, err
	}

	decoded, err := hex.DecodeString(host)
	if err != nil {
		return nil, fmt.Errorf("%q names none of the ports of a task this node holds", address)
	}

	var endpoint getendpoint.Response
	if err := json.Unmarshal(decoded, &endpoint); err != nil {
		return nil, fmt.Errorf("%q names none of the ports of a task this node holds: %w", address, err)
	}

	return &endpoint, nil
}
