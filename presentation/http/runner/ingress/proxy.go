// Package ingress serves the runner cluster's front door: a request naming an
// orchestrator is carried to that orchestrator, and one naming an orchestrator that is not there
// is answered as such.
package ingress

import (
	"context"
	"log/slog"
	"net/http"
	"net/http/httputil"
	"net/url"

	"github.com/khanzadimahdi/testproject/application/runner/ingress/checkOrchestratorExists"
	infraTrace "github.com/khanzadimahdi/testproject/infrastructure/telemetry/trace"
	"go.opentelemetry.io/otel/trace"
)

// proxyHandler forwards a request to the runner its path names.
//
// Everything an orchestrator serves is reached this way: an attached terminal, a log
// being followed, a plain call to its API. Nothing is dialled — the request goes
// back down a connection the orchestrator itself opened, which is what the transport
// resolves the runner's name to. An orchestrator therefore needs no address, and
// callers name one and are spared the question.
type proxyHandler struct {
	useCase *checkOrchestratorExists.UseCase
	proxy   *httputil.ReverseProxy
	logger  *slog.Logger
}

var _ http.Handler = &proxyHandler{}

// upstreamKey carries the resolved runner from ServeHTTP to the rewrite, which
// is the only hook a ReverseProxy gives for a per-request target.
type upstreamKey struct{}

func NewProxyHandler(
	useCase *checkOrchestratorExists.UseCase,
	transport http.RoundTripper, logger *slog.Logger,
) *proxyHandler {
	h := &proxyHandler{useCase: useCase, logger: logger}

	h.proxy = &httputil.ReverseProxy{
		Transport: transport,
		Rewrite: func(r *httputil.ProxyRequest) {
			upstream := r.In.Context().Value(upstreamKey{}).(*url.URL)

			r.SetURL(&url.URL{Scheme: upstream.Scheme, Host: upstream.Host})

			// set after SetURL, which joins the target's path onto the inbound
			// one: the orchestrator is asked for its own path, not for the ingress's.
			// SetURL also clears the outbound Host, which leaves the orchestrator
			// addressed by its name — it serves its own API here, not a site
			// that has to know what it is called.
			r.Out.URL.Path = upstream.Path
			r.Out.URL.RawQuery = r.In.URL.RawQuery

			r.SetXForwarded()
		},
		ErrorHandler: func(rw http.ResponseWriter, _ *http.Request, err error) {
			// the orchestrator is connected but nothing came back down it: its own
			// problem to report, not something the ingress can fix.
			h.logger.Error("could not reach an orchestrator", "error", err)
			rw.WriteHeader(http.StatusBadGateway)
		},
	}

	return h
}

// @Summary		Proxy to an orchestrator
// @Description	carries the request to the named orchestrator, path and all, including a websocket upgrade
// @Tags			orchestrator ingress
// @Param			name	path		string	true	"Orchestrator name"
// @Param			path	path		string	true	"Path on the orchestrator"
// @Success		200		{string}	string	"whatever the orchestrator answered"
// @Failure		404		{object}	map[string]interface{}
// @Failure		502		{object}	map[string]interface{}
// @Router			/orchestrators/{name}/{path} [get]
func (h *proxyHandler) ServeHTTP(rw http.ResponseWriter, r *http.Request) {
	request := checkOrchestratorExists.Request{
		Name: r.PathValue("name"),
	}

	exists, err := h.useCase.Execute(r.Context(), &request)

	switch {
	case err != nil:
		infraTrace.RecordError(trace.SpanFromContext(r.Context()), err)
		rw.WriteHeader(http.StatusInternalServerError)

		return
	case !exists:
		rw.WriteHeader(http.StatusNotFound)

		return
	default:
		// the host is the orchestrators rather than a machine: the transport resolves it
		// to one of the connections that orchestrator has open here.
		upstream := &url.URL{
			Scheme: "http",
			Host:   request.Name,
			Path:   "/" + r.PathValue("path"),
		}
		h.proxy.ServeHTTP(rw, r.WithContext(context.WithValue(r.Context(), upstreamKey{}, upstream)))
	}
}
