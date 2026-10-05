package ingress

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"net/http/httputil"
	"net/url"

	"github.com/khanzadimahdi/testproject/domain"
	"github.com/khanzadimahdi/testproject/domain/workload/ingress"
	infraTrace "github.com/khanzadimahdi/testproject/infrastructure/telemetry/trace"
	"go.opentelemetry.io/otel/trace"
)

// terminalHandler carries a terminal to the node holding a task or a VM.
//
// It works out which node that is and proxies the connection there, and that is
// all it does. Who may open a terminal is the node's to answer: it reads the
// owner off the task or the VM and compares it with the token carried in this
// request, neither of which the ingress looks at. So this is a pipe that knows
// an address, and nothing here has to be trusted for the answer to be right.
type terminalHandler struct {
	// what the terminal is opened in, as the answers name it.
	what string

	// locate is the node holding what uuid names, and route the node's own
	// route for its terminal.
	locate func(ctx context.Context, uuid string) (string, error)
	route  func(uuid string) string

	registry ingress.Registry

	proxy  *httputil.ReverseProxy
	logger *slog.Logger
}

var _ http.Handler = &terminalHandler{}

// NewTerminalHandler carries a terminal inside a task to the node holding the
// task, on the node's /api/tasks/{uuid}/attach.
func NewTerminalHandler(
	resolver Resolver,
	registry ingress.Registry,
	transport http.RoundTripper,
	logger *slog.Logger,
) *terminalHandler {
	locate := func(ctx context.Context, uuid string) (string, error) {
		t, err := resolver.GetOne(ctx, uuid)

		return t.NodeName, err
	}

	route := func(uuid string) string {
		return "/api/tasks/" + url.PathEscape(uuid) + "/attach"
	}

	return newTerminalHandler("task", locate, route, registry, transport, logger)
}

// NewVMTerminalHandler carries a terminal inside a VM to the node holding the
// VM, on the node's /vms/{uuid}/attach, exactly as a task's is carried.
func NewVMTerminalHandler(
	vms VMResolver,
	registry ingress.Registry,
	transport http.RoundTripper,
	logger *slog.Logger,
) *terminalHandler {
	locate := func(ctx context.Context, uuid string) (string, error) {
		v, err := vms.GetOne(ctx, uuid)

		return v.NodeName, err
	}

	route := func(uuid string) string {
		return "/vms/" + url.PathEscape(uuid) + "/attach"
	}

	return newTerminalHandler("vm", locate, route, registry, transport, logger)
}

func newTerminalHandler(
	what string,
	locate func(ctx context.Context, uuid string) (string, error),
	route func(uuid string) string,
	registry ingress.Registry,
	transport http.RoundTripper,
	logger *slog.Logger,
) *terminalHandler {
	h := &terminalHandler{what: what, locate: locate, route: route, registry: registry, logger: logger}

	h.proxy = &httputil.ReverseProxy{
		Transport: transport,
		Rewrite: func(r *httputil.ProxyRequest) {
			upstream := r.In.Context().Value(targetKey{}).(*url.URL)

			r.SetURL(&url.URL{Scheme: upstream.Scheme, Host: upstream.Host})

			// set after SetURL, which joins the target's path onto the inbound
			// one: the node is asked for its own route rather than for this.
			r.Out.URL.Path = upstream.Path
			r.Out.URL.RawQuery = r.In.URL.RawQuery

			r.SetXForwarded()
		},
		ErrorHandler: func(rw http.ResponseWriter, _ *http.Request, err error) {
			h.logger.Error("could not reach a node for a terminal", "error", err)
			rw.WriteHeader(http.StatusBadGateway)
		},
	}

	return h
}

// @Summary		Open a terminal in a task or a vm
// @Description	carries a websocket to the node holding the task or the vm, which decides who may open one
// @Tags			workload ingress
// @Param			uuid	path	string	true	"Task or VM UUID"
// @Success		101		{string}	string	"switching protocols"
// @Failure		404		{object}	map[string]interface{}
// @Failure		503		{object}	map[string]interface{}
// @Router			/tasks/{uuid}/attach [get]
// @Router			/vms/{uuid}/attach [get]
func (h *terminalHandler) ServeHTTP(rw http.ResponseWriter, r *http.Request) {
	uuid := r.PathValue("uuid")

	nodeName, err := h.locate(r.Context(), uuid)
	switch {
	case errors.Is(err, domain.ErrNotExists):
		http.Error(rw, "no such "+h.what, http.StatusNotFound)

		return
	case err != nil:
		infraTrace.RecordError(trace.SpanFromContext(r.Context()), err)
		rw.WriteHeader(http.StatusInternalServerError)

		return
	}

	if len(nodeName) == 0 {
		http.Error(rw, "the "+h.what+" has not been scheduled yet", http.StatusServiceUnavailable)

		return
	}

	// only the node holding it can open one, and only a node that is connected
	// can be asked to
	connected, err := h.registry.Exists(r.Context(), nodeName)
	if err != nil {
		infraTrace.RecordError(trace.SpanFromContext(r.Context()), err)
		rw.WriteHeader(http.StatusInternalServerError)

		return
	}

	if !connected {
		http.Error(rw, "the node holding this "+h.what+" is not connected", http.StatusServiceUnavailable)

		return
	}

	target := &url.URL{
		Scheme: "http",
		Host:   nodeName,
		Path:   h.route(uuid),
	}

	h.proxy.ServeHTTP(rw, r.WithContext(context.WithValue(r.Context(), targetKey{}, target)))
}
