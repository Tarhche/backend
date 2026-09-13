// Package ingress serves the runner cluster's front door: a request naming a
// worker is carried to that worker, and one naming a worker that is not there
// is answered as such.
package ingress

import (
	"context"
	"log/slog"
	"net/http"
	"net/http/httputil"
	"net/url"

	"github.com/khanzadimahdi/testproject/application/runner/ingress/checkWorkerExists"
	infraTrace "github.com/khanzadimahdi/testproject/infrastructure/telemetry/trace"
	"go.opentelemetry.io/otel/trace"
)

// proxyHandler forwards a request to the runner its path names.
//
// Everything a worker serves is reached this way: an attached terminal, a log
// being followed, a plain call to its API. Nothing is dialled — the request goes
// back down a connection the worker itself opened, which is what the transport
// resolves the runner's name to. A worker therefore needs no address, and
// callers name one and are spared the question.
type proxyHandler struct {
	useCase *checkWorkerExists.UseCase
	proxy   *httputil.ReverseProxy
	logger  *slog.Logger
}

var _ http.Handler = &proxyHandler{}

// upstreamKey carries the resolved runner from ServeHTTP to the rewrite, which
// is the only hook a ReverseProxy gives for a per-request target.
type upstreamKey struct{}

func NewProxyHandler(
	useCase *checkWorkerExists.UseCase,
	transport http.RoundTripper, logger *slog.Logger,
) *proxyHandler {
	h := &proxyHandler{useCase: useCase, logger: logger}

	h.proxy = &httputil.ReverseProxy{
		Transport: transport,
		Rewrite: func(r *httputil.ProxyRequest) {
			upstream := r.In.Context().Value(upstreamKey{}).(*url.URL)

			r.SetURL(&url.URL{Scheme: upstream.Scheme, Host: upstream.Host})

			// set after SetURL, which joins the target's path onto the inbound
			// one: the worker is asked for its own path, not for the ingress's.
			// SetURL also clears the outbound Host, which leaves the worker
			// addressed by its name — it serves its own API here, not a site
			// that has to know what it is called.
			r.Out.URL.Path = upstream.Path
			r.Out.URL.RawQuery = r.In.URL.RawQuery

			r.SetXForwarded()
		},
		ErrorHandler: func(rw http.ResponseWriter, _ *http.Request, err error) {
			// the worker is connected but nothing came back down it: its own
			// problem to report, not something the ingress can fix.
			h.logger.Error("could not reach a worker", "error", err)
			rw.WriteHeader(http.StatusBadGateway)
		},
	}

	return h
}

// @Summary		Proxy to a worker
// @Description	carries the request to the named worker, path and all, including a websocket upgrade
// @Tags			worker ingress
// @Param			name	path		string	true	"Worker name"
// @Param			path	path		string	true	"Path on the worker"
// @Success		200		{string}	string	"whatever the worker answered"
// @Failure		404		{object}	map[string]interface{}
// @Failure		502		{object}	map[string]interface{}
// @Router			/workers/{name}/{path} [get]
func (h *proxyHandler) ServeHTTP(rw http.ResponseWriter, r *http.Request) {
	request := checkWorkerExists.Request{
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
		// the host is the workers rather than a machine: the transport resolves it
		// to one of the connections that worker has open here.
		upstream := &url.URL{
			Scheme: "http",
			Host:   request.Name,
			Path:   "/" + r.PathValue("path"),
		}
		h.proxy.ServeHTTP(rw, r.WithContext(context.WithValue(r.Context(), upstreamKey{}, upstream)))
	}
}
