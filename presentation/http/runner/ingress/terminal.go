package ingress

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"net/http/httputil"
	"net/url"

	"github.com/khanzadimahdi/testproject/domain"
	"github.com/khanzadimahdi/testproject/domain/runner/ingress"
	infraTrace "github.com/khanzadimahdi/testproject/infrastructure/telemetry/trace"
	"go.opentelemetry.io/otel/trace"
)

// TerminalPath is where a node serves a terminal into one of the containers it
// is holding. The ingress builds it and the node routes it, so it is written
// down once, here.
const TerminalPath = "/api/tasks/%s/attach"

// terminalHandler carries a terminal to the node holding the container.
//
// It works out which node that is and proxies the connection there, and that is
// all it does. Who may open a terminal is the node's to answer: it reads the
// owner off the container and compares it with the token carried in this
// request, neither of which the ingress looks at. So this is a pipe that knows
// an address, and nothing here has to be trusted for the answer to be right.
type terminalHandler struct {
	resolver Resolver
	registry ingress.Registry

	proxy  *httputil.ReverseProxy
	logger *slog.Logger
}

var _ http.Handler = &terminalHandler{}

func NewTerminalHandler(
	resolver Resolver,
	registry ingress.Registry,
	transport http.RoundTripper,
	logger *slog.Logger,
) *terminalHandler {
	h := &terminalHandler{resolver: resolver, registry: registry, logger: logger}

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

// @Summary		Open a terminal in a container
// @Description	carries a websocket to the node holding the container, which decides who may open one
// @Tags			runner ingress
// @Param			uuid	path	string	true	"Container UUID"
// @Success		101		{string}	string	"switching protocols"
// @Failure		404		{object}	map[string]interface{}
// @Failure		503		{object}	map[string]interface{}
// @Router			/containers/{uuid}/attach [get]
func (h *terminalHandler) ServeHTTP(rw http.ResponseWriter, r *http.Request) {
	uuid := r.PathValue("uuid")

	t, err := h.resolver.GetOne(r.Context(), uuid)
	switch {
	case errors.Is(err, domain.ErrNotExists):
		http.Error(rw, "no such container", http.StatusNotFound)

		return
	case err != nil:
		infraTrace.RecordError(trace.SpanFromContext(r.Context()), err)
		rw.WriteHeader(http.StatusInternalServerError)

		return
	}

	if len(t.NodeName) == 0 {
		http.Error(rw, "the container has not been scheduled yet", http.StatusServiceUnavailable)

		return
	}

	// only the node holding it can open one, and only a node that is connected
	// can be asked to
	connected, err := h.registry.Exists(r.Context(), t.NodeName)
	if err != nil {
		infraTrace.RecordError(trace.SpanFromContext(r.Context()), err)
		rw.WriteHeader(http.StatusInternalServerError)

		return
	}

	if !connected {
		http.Error(rw, "the node holding this container is not connected", http.StatusServiceUnavailable)

		return
	}

	target := &url.URL{
		Scheme: "http",
		Host:   t.NodeName,
		Path:   "/api/tasks/" + url.PathEscape(uuid) + "/attach",
	}

	h.proxy.ServeHTTP(rw, r.WithContext(context.WithValue(r.Context(), targetKey{}, target)))
}
