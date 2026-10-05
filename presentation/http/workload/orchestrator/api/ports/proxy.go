// Package ports serves the ports of the VMs and the code-runner tasks this
// node holds.
//
// The ingress cannot see a VM or a task: it works out which node holds one and
// sends the request here. So this is the far end of that — the node reaching a
// port where its engine published it, and saying so itself when it cannot.
package ports

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"net/http/httputil"
	"net/url"
	"strconv"

	"go.opentelemetry.io/otel/trace"

	getendpoint "github.com/khanzadimahdi/testproject/application/workload/orchestrator/getEndpoint"
	"github.com/khanzadimahdi/testproject/domain/workload/port"
	infraTrace "github.com/khanzadimahdi/testproject/infrastructure/telemetry/trace"
)

// idleConnectionsPerPort is how many connections to one published port are
// kept for the next request. An engine takes only so many new connections to
// a VM's port in a while — microsandbox about 256 every ten seconds — so a
// port being browsed is reached over connections kept open rather than a new
// one for each request.
const idleConnectionsPerPort = 64

type proxyHandler struct {
	useCase *getendpoint.UseCase
	proxy   *httputil.ReverseProxy
	logger  *slog.Logger
}

var _ http.Handler = &proxyHandler{}

func NewProxyHandler(useCase *getendpoint.UseCase, logger *slog.Logger) *proxyHandler {
	h := &proxyHandler{useCase: useCase, logger: logger}

	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.MaxIdleConnsPerHost = idleConnectionsPerPort

	h.proxy = &httputil.ReverseProxy{
		Transport: transport,
		Rewrite: func(r *httputil.ProxyRequest) {
			upstream := r.In.Context().Value(targetKey{}).(*url.URL)

			r.SetURL(&url.URL{Scheme: upstream.Scheme, Host: upstream.Host})

			// set after SetURL: what is asked for is the path the client asked
			// the ingress for, not the route it arrived here on.
			r.Out.URL.Path = upstream.Path
			r.Out.URL.RawQuery = r.In.URL.RawQuery

			// it is addressed by the name the client used, not by the port it
			// happens to be published on.
			r.Out.Host = r.In.Host
		},
		ErrorHandler: func(rw http.ResponseWriter, _ *http.Request, err error) {
			// it is there but not answering: its own problem to report, not
			// something the workload can fix.
			h.logger.Error("could not reach a port this node is holding", "error", err)
			http.Error(rw, "it is not answering", http.StatusBadGateway)
		},
	}

	return h
}

// targetKey carries the resolved upstream from ServeHTTP to the rewrite, which
// is the only hook a ReverseProxy gives for a per-request target.
type targetKey struct{}

// @Summary		Serve a VM's or a task's port
// @Description	carries the request to a port of a VM or a task this node is holding, where its engine published it
// @Tags			workload
// @Param			slug	path		string	true	"VM or task slug"
// @Param			port	path		int		true	"Its port, or 0 for the lowest it exposes"
// @Param			path	path		string	true	"Path on it"
// @Success		200		{string}	string	"whatever it answered"
// @Failure		404		{object}	map[string]interface{}
// @Failure		502		{object}	map[string]interface{}
// @Failure		503		{object}	map[string]interface{}
// @Router			/tasks/{slug}/{port}/{path} [get]
// @Router			/vms/{slug}/{port}/{path} [get]
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
	case errors.Is(err, getendpoint.ErrNotHeld), errors.Is(err, getendpoint.ErrNotExposed):
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
		Host:   response.Address,
		Path:   "/" + r.PathValue("path"),
	}

	h.proxy.ServeHTTP(rw, r.WithContext(context.WithValue(r.Context(), targetKey{}, target)))
}
