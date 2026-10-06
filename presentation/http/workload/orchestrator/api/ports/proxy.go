// Package ports serves the ports of the code-runner tasks and of the
// resources of every kind with endpoints this node holds, VMs among them.
//
// The ingress cannot see any of them: it works out which node holds one and
// sends the request here. So this is the far end of that — the node reaching a
// port where it was published, and saying so itself when it cannot.
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
	getresourceendpoint "github.com/khanzadimahdi/testproject/application/workload/orchestrator/getResourceEndpoint"
	"github.com/khanzadimahdi/testproject/domain"
	"github.com/khanzadimahdi/testproject/domain/workload/kind"
	"github.com/khanzadimahdi/testproject/domain/workload/port"
	infraTrace "github.com/khanzadimahdi/testproject/infrastructure/telemetry/trace"
)

// idleConnectionsPerPort is how many connections to one published port are
// kept for the next request. An engine takes only so many new connections to
// a VM's port in a while — microsandbox about 256 every ten seconds — so a
// port being browsed is reached over connections kept open rather than a new
// one for each request.
const idleConnectionsPerPort = 64

// resolver is the address a request for port p of what slug names is sent
// to.
type resolver func(ctx context.Context, slug string, p port.Port) (address string, err error)

type proxyHandler struct {
	resolve resolver
	proxy   *httputil.ReverseProxy
	logger  *slog.Logger
}

var _ http.Handler = &proxyHandler{}

// NewProxyHandler serves the ports of the tasks this node holds, wherever its
// engine published them.
func NewProxyHandler(useCase *getendpoint.UseCase, logger *slog.Logger) *proxyHandler {
	return newProxyHandler(func(ctx context.Context, slug string, p port.Port) (string, error) {
		response, err := useCase.Execute(ctx, &getendpoint.Request{Slug: slug, Port: p})
		if err != nil {
			return "", err
		}

		return response.Address, nil
	}, logger)
}

// NewResourceProxyHandler serves the ports of the resources of one kind this
// node holds, wherever the kind's node strategy says they are.
func NewResourceProxyHandler(useCase *getresourceendpoint.UseCase, kindName string, logger *slog.Logger) *proxyHandler {
	return newProxyHandler(func(ctx context.Context, slug string, p port.Port) (string, error) {
		response, err := useCase.Execute(ctx, &getresourceendpoint.Request{Kind: kindName, Slug: slug, Port: p})
		if err != nil {
			return "", err
		}

		return response.Address, nil
	}, logger)
}

func newProxyHandler(resolve resolver, logger *slog.Logger) *proxyHandler {
	h := &proxyHandler{resolve: resolve, logger: logger}

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

// @Summary		Serve a task's or a resource's port
// @Description	carries the request to a port of a task, or of a resource of a kind with endpoints such as a VM, this node is holding, where it was published
// @Tags			workload
// @Param			slug	path		string	true	"Task or resource slug"
// @Param			port	path		int		true	"Its port, or 0 for the lowest it exposes"
// @Param			path	path		string	true	"Path on it"
// @Success		200		{string}	string	"whatever it answered"
// @Failure		404		{object}	map[string]interface{}
// @Failure		502		{object}	map[string]interface{}
// @Failure		503		{object}	map[string]interface{}
// @Router			/tasks/{slug}/{port}/{path} [get]
// @Router			/{plural}/{slug}/{port}/{path} [get]
func (h *proxyHandler) ServeHTTP(rw http.ResponseWriter, r *http.Request) {
	requested, err := strconv.ParseUint(r.PathValue("port"), 10, 16)
	if err != nil {
		http.Error(rw, "that is not a port", http.StatusBadRequest)

		return
	}

	address, err := h.resolve(r.Context(), r.PathValue("slug"), port.Port(requested))

	switch {
	case errors.Is(err, getendpoint.ErrNotHeld), errors.Is(err, getendpoint.ErrNotExposed), errors.Is(err, domain.ErrNotExists):
		http.Error(rw, err.Error(), http.StatusNotFound)

		return
	case errors.Is(err, getendpoint.ErrNotRunning), errors.Is(err, kind.ErrUnreachable):
		http.Error(rw, err.Error(), http.StatusServiceUnavailable)

		return
	case err != nil:
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
