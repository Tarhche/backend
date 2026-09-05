// Package ingress serves the ports containers expose. A container's slug is the
// left-most label of the hostname it answers on, so "nginx-xkfqz" reaches the
// container's lowest exposed port and "nginx-xkfqz-8080" reaches port 8080 of
// the same container.
package ingress

import (
	"context"
	"net"
	"net/http"
	"net/http/httputil"
	"net/url"
	"slices"
	"strconv"
	"strings"

	"github.com/khanzadimahdi/testproject/domain/runner/port"
	"github.com/khanzadimahdi/testproject/domain/runner/task"
)

// Resolver finds the container a hostname names. It is the task repository in
// production; the ingress asks for no more than this so it can be driven by a
// double in tests.
type Resolver interface {
	GetOneBySlug(ctx context.Context, slug string) (task.Task, error)
}

// Handler proxies a request to the container its hostname names.
type Handler struct {
	resolver Resolver

	// domain is the suffix every container hostname carries, without a leading
	// dot: "runner.tarhche.com", or "runner.localhost" while developing.
	domain string

	proxy *httputil.ReverseProxy
}

var _ http.Handler = &Handler{}

func NewHandler(resolver Resolver, domain string) *Handler {
	h := &Handler{
		resolver: resolver,
		domain:   strings.ToLower(strings.Trim(domain, ".")),
	}

	h.proxy = &httputil.ReverseProxy{
		Rewrite: func(r *httputil.ProxyRequest) {
			r.SetURL(r.In.Context().Value(targetKey{}).(*url.URL))

			// the container is addressed by the name the client used, not by
			// the node and port it happens to sit on.
			r.Out.Host = r.In.Host

			r.SetXForwarded()
		},
		ErrorHandler: func(rw http.ResponseWriter, _ *http.Request, _ error) {
			// the container is there but not answering: its own problem to
			// report, not something the runner can fix.
			rw.WriteHeader(http.StatusBadGateway)
		},
	}

	return h
}

// targetKey carries the resolved upstream from ServeHTTP to the rewrite, which
// is the only hook a ReverseProxy gives for a per-request target.
type targetKey struct{}

func (h *Handler) ServeHTTP(rw http.ResponseWriter, r *http.Request) {
	slug, containerPort, ok := h.parseHost(r.Host)
	if !ok {
		http.Error(rw, "unknown container", http.StatusNotFound)

		return
	}

	t, err := h.resolver.GetOneBySlug(r.Context(), slug)
	if err != nil {
		http.Error(rw, "unknown container", http.StatusNotFound)

		return
	}

	endpoint, found := selectEndpoint(t.Endpoints, containerPort)
	if !found {
		http.Error(rw, "the container does not expose that port", http.StatusNotFound)

		return
	}

	if t.CurrentState != task.Running {
		http.Error(rw, "the container is not running", http.StatusServiceUnavailable)

		return
	}

	target := &url.URL{
		Scheme: "http",
		Host:   net.JoinHostPort(endpoint.Host, strconv.FormatUint(uint64(endpoint.HostPort), 10)),
	}

	h.proxy.ServeHTTP(rw, r.WithContext(context.WithValue(r.Context(), targetKey{}, target)))
}

// parseHost takes the container's slug, and optionally the port it names, out
// of a hostname. A hostname outside the ingress domain names no container.
//
// The slug's own random suffix is letters only, so a trailing group of digits
// can only ever be a port.
func (h *Handler) parseHost(host string) (string, port.Port, bool) {
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

// selectEndpoint picks the endpoint a hostname asked for. A hostname naming no
// port reaches the lowest one the container exposes, so the common case of a
// container with a single port needs no port in its name at all.
func selectEndpoint(endpoints []task.Endpoint, requested port.Port) (task.Endpoint, bool) {
	if len(endpoints) == 0 {
		return task.Endpoint{}, false
	}

	if requested > 0 {
		for _, endpoint := range endpoints {
			if endpoint.ContainerPort == requested {
				return endpoint, endpoint.HostPort > 0
			}
		}

		return task.Endpoint{}, false
	}

	lowest := slices.MinFunc(endpoints, func(a task.Endpoint, b task.Endpoint) int {
		return int(a.ContainerPort) - int(b.ContainerPort)
	})

	return lowest, lowest.HostPort > 0
}
