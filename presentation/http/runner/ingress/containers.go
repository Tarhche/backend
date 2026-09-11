package ingress

import (
	"context"
	"errors"
	"log/slog"
	"net"
	"net/http"
	"net/http/httputil"
	"net/url"
	"strconv"
	"strings"

	"github.com/khanzadimahdi/testproject/domain"
	"github.com/khanzadimahdi/testproject/domain/runner/ingress"
	"github.com/khanzadimahdi/testproject/domain/runner/port"
	"github.com/khanzadimahdi/testproject/domain/runner/task"
	infraTrace "github.com/khanzadimahdi/testproject/infrastructure/telemetry/trace"
	"go.opentelemetry.io/otel/trace"
)

// ContainerPath is where a node serves one of the containers it is holding. The
// ingress builds it and the node routes it; they are the two ends of the same
// agreement, so they are written down once, here.
const ContainerPath = "/containers/%s/%d/%s"

// Resolver finds which node is holding the container a hostname names. It is the
// task repository in production; the ingress asks for no more than this so it
// can be driven by a double in tests.
type Resolver interface {
	GetOneBySlug(ctx context.Context, slug string) (task.Task, error)
}

// containerHandler serves the ports containers expose. A container's slug is
// the left-most label of the hostname it answers on, so "nginx-xkfqz" reaches
// the container's lowest exposed port and "nginx-xkfqz-8080" reaches port 8080
// of the same container.
//
// The ingress cannot see a container — only the node holding one can — so it
// works out which node that is and sends the request down that node's own
// connection. What the node finds there is the node's to report.
type containerHandler struct {
	resolver Resolver
	registry ingress.Registry

	// domain is the suffix every container hostname carries, without a leading
	// dot: "runner.tarhche.com", or "runner.localhost" while developing.
	domain string

	proxy  *httputil.ReverseProxy
	logger *slog.Logger
}

var _ http.Handler = &containerHandler{}

func NewContainerHandler(
	resolver Resolver,
	registry ingress.Registry,
	transport http.RoundTripper,
	domain string,
	logger *slog.Logger,
) *containerHandler {
	h := &containerHandler{
		resolver: resolver,
		registry: registry,
		domain:   strings.ToLower(strings.Trim(domain, ".")),
		logger:   logger,
	}

	h.proxy = &httputil.ReverseProxy{
		Transport: transport,
		Rewrite: func(r *httputil.ProxyRequest) {
			upstream := r.In.Context().Value(targetKey{}).(*url.URL)

			r.SetURL(&url.URL{Scheme: upstream.Scheme, Host: upstream.Host})

			// set after SetURL, which joins the target's path onto the inbound
			// one. The node is asked for its own route; what the container is
			// asked for rides inside it.
			r.Out.URL.Path = upstream.Path
			r.Out.URL.RawQuery = r.In.URL.RawQuery

			// the container is addressed by the name the client used, not by
			// the node it happens to sit on.
			r.Out.Host = r.In.Host

			r.SetXForwarded()
		},
		ErrorHandler: func(rw http.ResponseWriter, _ *http.Request, err error) {
			// the node took the request and nothing came back: its own problem
			// to report, not something the ingress can fix.
			h.logger.Error("could not reach the node holding a container", "error", err)
			rw.WriteHeader(http.StatusBadGateway)
		},
	}

	return h
}

// targetKey carries the resolved upstream from ServeHTTP to the rewrite, which
// is the only hook a ReverseProxy gives for a per-request target.
type targetKey struct{}

func (h *containerHandler) ServeHTTP(rw http.ResponseWriter, r *http.Request) {
	slug, containerPort, ok := h.parseHost(r.Host)
	if !ok {
		http.Error(rw, "unknown container", http.StatusNotFound)

		return
	}

	t, err := h.resolver.GetOneBySlug(r.Context(), slug)
	switch {
	case errors.Is(err, domain.ErrNotExists):
		http.Error(rw, "unknown container", http.StatusNotFound)

		return
	case err != nil:
		infraTrace.RecordError(trace.SpanFromContext(r.Context()), err)
		rw.WriteHeader(http.StatusInternalServerError)

		return
	}

	if t.State != task.Running {
		http.Error(rw, "the container is not running", http.StatusServiceUnavailable)

		return
	}

	if len(t.NodeName) == 0 {
		http.Error(rw, "the container has not been scheduled yet", http.StatusServiceUnavailable)

		return
	}

	// only the node holding it can reach a container, and only a node that is
	// connected can be asked to.
	if _, err := h.registry.Get(r.Context(), t.NodeName); err != nil {
		http.Error(rw, "the node holding this container is not connected", http.StatusServiceUnavailable)

		return
	}

	target := &url.URL{
		Scheme: "http",
		Host:   t.NodeName,
		Path: strings.Join([]string{
			"/containers",
			url.PathEscape(slug),
			strconv.FormatUint(uint64(containerPort), 10),
			strings.TrimPrefix(r.URL.Path, "/"),
		}, "/"),
	}

	h.proxy.ServeHTTP(rw, r.WithContext(context.WithValue(r.Context(), targetKey{}, target)))
}

// parseHost takes the container's slug, and optionally the port it names, out
// of a hostname. A hostname outside the ingress domain names no container.
//
// The slug's own random suffix is letters only, so a trailing group of digits
// can only ever be a port.
func (h *containerHandler) parseHost(host string) (string, port.Port, bool) {
	name := strings.ToLower(host)

	if hostname, _, err := net.SplitHostPort(host); err == nil {
		name = strings.ToLower(hostname)
	}

	label, rest, found := strings.Cut(name, ".")
	if !found || rest != h.domain || len(label) == 0 {
		return "", 0, false
	}

	index := strings.LastIndex(label, "-")
	if index <= 0 {
		return label, 0, true
	}

	requested, err := strconv.ParseUint(label[index+1:], 10, 16)
	if err != nil || requested == 0 {
		return label, 0, true
	}

	return label[:index], port.Port(requested), true
}
