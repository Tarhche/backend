// Package ingress serves the ports containers expose. A container's slug is the
// left-most label of the hostname it answers on, so "nginx-xkfqz" reaches the
// container's lowest exposed port and "nginx-xkfqz-8080" reaches port 8080 of
// the same container.
package ingress

import (
	"context"
	"fmt"
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
		ErrorHandler: func(rw http.ResponseWriter, r *http.Request, _ error) {
			// the container is there but not answering, which for a container
			// that has only just started usually means it is still coming up.
			// Whoever is looking at it is served a page that comes back on its
			// own; anything else is told plainly that it is a bad gateway.
			writeStarting(rw, r)
		},
	}

	return h
}

// startingSeconds is how long a page that is waiting for a container waits
// before asking again.
const startingSeconds = 2

// startingPage is what a browser is shown while a container is not answering
// yet: it says so, and comes back on its own until it is. What it says is
// filled in from the language the browser asked for.
const startingPage = `<!doctype html>
<html lang="%s" dir="%s">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width,initial-scale=1">
<meta http-equiv="refresh" content="2">
<title>starting…</title>
<style>
  :root { color-scheme: light dark; }
  body {
    margin: 0;
    height: 100vh;
    display: flex;
    flex-direction: column;
    align-items: center;
    justify-content: center;
    gap: 14px;
    perspective: 200px;
    background: #fff;
    color: #868e96;
    font: 13px/1.5 ui-sans-serif, system-ui, sans-serif;
  }
  .cube { position: relative; width: 18px; height: 18px; transform-style: preserve-3d; animation: turn 3s infinite linear; }
  .cube span { position: absolute; inset: 0; border: 1.5px solid #228be6; opacity: .85; }
  .cube span:nth-child(1) { transform: translateZ(9px); }
  .cube span:nth-child(2) { transform: rotateY(180deg) translateZ(9px); }
  .cube span:nth-child(3) { transform: rotateY(90deg) translateZ(9px); }
  .cube span:nth-child(4) { transform: rotateY(-90deg) translateZ(9px); }
  .cube span:nth-child(5) { transform: rotateX(90deg) translateZ(9px); }
  .cube span:nth-child(6) { transform: rotateX(-90deg) translateZ(9px); }
  @keyframes turn { from { transform: rotateX(-24deg) rotateY(0); } to { transform: rotateX(-24deg) rotateY(360deg); } }
  @media (prefers-reduced-motion: reduce) { .cube { animation-duration: 0s; } }
  @media (prefers-color-scheme: dark) { body { background: #1a1b1e; color: #909296; } }
</style>
</head>
<body>
  <div class="cube"><span></span><span></span><span></span><span></span><span></span><span></span></div>
  <p>%s</p>
</body>
</html>
`

// writeStarting answers a request for a container that is not answering yet.
// A browser is given the waiting page, which asks again on its own; anything
// else — a fetch, a health check, a command line — is given the bare status it
// can act on.
// starting is what the waiting page says, in the language the browser asked
// for. It knows the two the site is written in and falls back to English,
// which is what the runner itself speaks.
func starting(acceptLanguage string) (lang string, dir string, text string) {
	if strings.HasPrefix(strings.TrimSpace(strings.ToLower(acceptLanguage)), "fa") {
		return "fa", "rtl", "در حال آماده‌سازی…"
	}

	return "en", "ltr", "starting…"
}

func writeStarting(rw http.ResponseWriter, r *http.Request) {
	rw.Header().Set("Retry-After", strconv.Itoa(startingSeconds))
	rw.Header().Set("Cache-Control", "no-store")

	if !strings.Contains(r.Header.Get("Accept"), "text/html") {
		http.Error(rw, "the container is not answering", http.StatusBadGateway)

		return
	}

	lang, dir, text := starting(r.Header.Get("Accept-Language"))

	rw.Header().Set("Content-Type", "text/html; charset=utf-8")
	rw.Header().Set("Vary", "Accept-Language")
	rw.WriteHeader(http.StatusBadGateway)
	_, _ = fmt.Fprintf(rw, startingPage, lang, dir, text)
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
