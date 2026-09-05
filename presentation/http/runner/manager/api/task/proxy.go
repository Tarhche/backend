package task

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"net/http/httputil"
	"net/url"
	"strings"

	managerGetNode "github.com/khanzadimahdi/testproject/application/runner/manager/node/getNode"
	gettask "github.com/khanzadimahdi/testproject/application/runner/manager/task/getTask"
	"github.com/khanzadimahdi/testproject/domain"
	infraTrace "github.com/khanzadimahdi/testproject/infrastructure/telemetry/trace"
	"go.opentelemetry.io/otel/trace"
)

// nodeProxy forwards a request to the node running a task.
//
// A terminal and a live log are streams rather than answers, and the only place
// they exist is on the node holding the container. The manager is what knows
// which node that is, so it stands in front and passes the connection on —
// including a websocket upgrade, which a reverse proxy carries through.
type nodeProxy struct {
	tasks *gettask.UseCase
	nodes *managerGetNode.UseCase
	path  string
	proxy *httputil.ReverseProxy

	logger *slog.Logger
}

var _ http.Handler = &nodeProxy{}

// newNodeProxy builds a proxy that forwards to path on the task's own node,
// with "{uuid}" in path replaced by the task's uuid.
func newNodeProxy(tasks *gettask.UseCase, nodes *managerGetNode.UseCase, path string, logger *slog.Logger) *nodeProxy {
	p := &nodeProxy{tasks: tasks, nodes: nodes, path: path, logger: logger}

	p.proxy = &httputil.ReverseProxy{
		Rewrite: func(r *httputil.ProxyRequest) {
			upstream := r.In.Context().Value(upstreamKey{}).(*url.URL)

			r.SetURL(&url.URL{Scheme: upstream.Scheme, Host: upstream.Host})

			// set after SetURL, which joins the target's path onto the inbound
			// one. The node is asked for its own route, not for the manager's.
			r.Out.URL.Path = upstream.Path
			r.Out.URL.RawQuery = r.In.URL.RawQuery
			r.Out.Host = r.In.Host
		},
		ErrorHandler: func(rw http.ResponseWriter, _ *http.Request, err error) {
			p.logger.Error("could not reach the node holding a container", "error", err)
			rw.WriteHeader(http.StatusBadGateway)
		},
	}

	return p
}

type upstreamKey struct{}

func (p *nodeProxy) ServeHTTP(rw http.ResponseWriter, r *http.Request) {
	uuid := r.PathValue("uuid")

	t, err := p.tasks.Execute(r.Context(), uuid)
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

	n, err := p.nodes.Execute(r.Context(), &managerGetNode.Request{Name: t.NodeName})
	switch {
	case errors.Is(err, domain.ErrNotExists):
		http.Error(rw, "the node holding the container is gone", http.StatusServiceUnavailable)

		return
	case err != nil:
		infraTrace.RecordError(trace.SpanFromContext(r.Context()), err)
		rw.WriteHeader(http.StatusInternalServerError)

		return
	}

	if len(n.APIAddress) == 0 {
		http.Error(rw, "the node holding the container cannot be reached", http.StatusServiceUnavailable)

		return
	}

	upstream := &url.URL{
		Scheme: "http",
		Host:   n.APIAddress,
		Path:   strings.ReplaceAll(p.path, "{uuid}", url.PathEscape(uuid)),
	}

	p.proxy.ServeHTTP(rw, r.WithContext(context.WithValue(r.Context(), upstreamKey{}, upstream)))
}

// NewAttachHandler proxies a terminal to the node running the container.
//
// @Summary		Attach to a task
// @Description	upgrades to a websocket carrying a command running inside the container: binary frames are its input and output, and a text frame resizes its terminal
// @Tags			runner tasks
// @Param			uuid	path	string	true	"Task UUID"
// @Success		101		{string}	string	"switching protocols"
// @Failure		404		{object}	map[string]interface{}
// @Failure		503		{object}	map[string]interface{}
// @Router			/tasks/{uuid}/attach [get]
func NewAttachHandler(tasks *gettask.UseCase, nodes *managerGetNode.UseCase, logger *slog.Logger) http.Handler {
	return newNodeProxy(tasks, nodes, "/api/tasks/{uuid}/attach", logger)
}
