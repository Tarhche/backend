// Package ingress serves the runner cluster's front door: a request naming a
// runner is carried to that runner, and one naming a runner that is not there
// is answered as such.
package ingress

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"net/http/httputil"
	"net/url"

	getRunner "github.com/khanzadimahdi/testproject/application/runner/ingress/getRunner"
	"github.com/khanzadimahdi/testproject/domain"
	infraTrace "github.com/khanzadimahdi/testproject/infrastructure/telemetry/trace"
	"go.opentelemetry.io/otel/trace"
)

// proxyHandler forwards a request to the runner its path names.
//
// Everything a worker serves is reached this way: an attached terminal, a log
// being followed, a plain call to its API. Nothing is dialled — the request goes
// back down a connection the worker itself opened, which is what the transport
// resolves the runner's id to. A worker therefore needs no address, and callers
// name one by its id and are spared the question.
type proxyHandler struct {
	useCase *getRunner.UseCase
	proxy   *httputil.ReverseProxy
	logger  *slog.Logger
}

var _ http.Handler = &proxyHandler{}

func NewProxyHandler(useCase *getRunner.UseCase, transport http.RoundTripper, logger *slog.Logger) *proxyHandler {
	h := &proxyHandler{useCase: useCase, logger: logger}

	h.proxy = &httputil.ReverseProxy{
		Transport: transport,
		Rewrite: func(r *httputil.ProxyRequest) {
			upstream := r.In.Context().Value(upstreamKey{}).(*url.URL)

			r.SetURL(&url.URL{Scheme: upstream.Scheme, Host: upstream.Host})

			// set after SetURL, which joins the target's path onto the inbound
			// one: the runner is asked for its own path, not for the ingress's.
			// SetURL also clears the outbound Host, which leaves the runner
			// addressed by its id — it serves its own API here, not a site that
			// has to know what it is called.
			r.Out.URL.Path = upstream.Path
			r.Out.URL.RawQuery = r.In.URL.RawQuery

			r.SetXForwarded()
		},
		ErrorHandler: func(rw http.ResponseWriter, _ *http.Request, err error) {
			// the runner is connected but nothing came back down it: its own
			// problem to report, not something the ingress can fix.
			h.logger.Error("could not reach a runner", "error", err)
			rw.WriteHeader(http.StatusBadGateway)
		},
	}

	return h
}

// upstreamKey carries the resolved runner from ServeHTTP to the rewrite, which
// is the only hook a ReverseProxy gives for a per-request target.
type upstreamKey struct{}

// @Summary		Proxy to a runner
// @Description	carries the request to the runner the id names, path and all, including a websocket upgrade
// @Tags			runner ingress
// @Param			id		path		string	true	"Runner id"
// @Param			path	path		string	true	"Path on the runner"
// @Success		200		{string}	string	"whatever the runner answered"
// @Failure		404		{object}	map[string]interface{}
// @Failure		502		{object}	map[string]interface{}
// @Router			/runners/{id}/{path} [get]
func (h *proxyHandler) ServeHTTP(rw http.ResponseWriter, r *http.Request) {
	request := getRunner.Request{
		ID: r.PathValue("id"),
	}

	response, err := h.useCase.Execute(r.Context(), &request)

	switch {
	case errors.Is(err, domain.ErrNotExists):
		http.Error(rw, "unknown runner", http.StatusNotFound)

		return
	case err != nil:
		infraTrace.RecordError(trace.SpanFromContext(r.Context()), err)
		rw.WriteHeader(http.StatusInternalServerError)

		return
	}

	// the host is the runner rather than a machine: the transport resolves it
	// to one of the connections that runner has open here.
	upstream := &url.URL{
		Scheme: "http",
		Host:   response.ID,
		Path:   "/" + r.PathValue("path"),
	}

	h.proxy.ServeHTTP(rw, r.WithContext(context.WithValue(r.Context(), upstreamKey{}, upstream)))
}
