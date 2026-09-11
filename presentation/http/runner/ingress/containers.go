package ingress

import (
	"context"
	"errors"
	"fmt"
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
		ErrorHandler: func(rw http.ResponseWriter, r *http.Request, err error) {
			// the node took the request and nothing came back, which for a
			// container that has only just started usually means it is still
			// coming up. Whoever is looking at it is served a page that comes
			// back on its own; anything else is told plainly.
			h.logger.Error("could not reach the node holding a container", "error", err)
			writeStarting(rw, r)
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

	if t.CurrentState != task.Running {
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
