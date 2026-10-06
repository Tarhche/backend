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
	"slices"
	"strconv"
	"strings"

	"github.com/khanzadimahdi/testproject/domain"
	"github.com/khanzadimahdi/testproject/domain/workload/ingress"
	"github.com/khanzadimahdi/testproject/domain/workload/kind"
	"github.com/khanzadimahdi/testproject/domain/workload/port"
	"github.com/khanzadimahdi/testproject/domain/workload/task"
	"github.com/khanzadimahdi/testproject/domain/workload/vm"
	infraTrace "github.com/khanzadimahdi/testproject/infrastructure/telemetry/trace"
	"go.opentelemetry.io/otel/trace"
)

// Resolver finds which node is holding a task, by the slug a hostname
// carries or by the uuid a terminal asks for. It is the task repository in
// production; the ingress asks for no more than this so it can be driven by a
// double in tests.
type Resolver interface {
	GetOneBySlug(ctx context.Context, slug string) (task.Task, error)
	GetOne(ctx context.Context, uuid string) (task.Task, error)
}

// VMResolver finds which node is holding a VM, by the slug a hostname carries
// or by the uuid a terminal asks for. It is the VM repository in production.
type VMResolver interface {
	GetOneBySlug(ctx context.Context, slug string) (vm.VM, error)
	GetOne(ctx context.Context, uuid string) (vm.VM, error)
}

// taskHandler serves the ports VMs, tasks and the resources of kinds with
// endpoints expose. A slug is unique across them, so it is looked up among the
// VMs first, the tasks second, and then among the kinds with endpoints, in the
// order they were registered. A task's slug is the left-most label of the
// hostname it answers on, so "nginx-xkfqz" reaches the task's lowest exposed
// port and "nginx-xkfqz-8080" reaches port 8080 of the same task.
//
// The ingress cannot see a task — only the node holding one can — so it
// works out which node that is and sends the request down that node's own
// connection. What the node finds there is the node's to report.
type taskHandler struct {
	resolver Resolver
	vms      VMResolver
	registry ingress.Registry

	// kinds are the kinds whose resources the ingress finds, of which those
	// with endpoints are asked for a slug that is neither a VM's nor a
	// task's.
	kinds *kind.Registry[kind.IngressBinding]

	// domain is the suffix every task hostname carries, without a leading
	// dot: "workload.tarhche.com", or "workload.localhost" while developing.
	domain string

	proxy  *httputil.ReverseProxy
	logger *slog.Logger
}

var _ http.Handler = &taskHandler{}

func NewTaskHandler(
	resolver Resolver,
	vms VMResolver,
	kinds *kind.Registry[kind.IngressBinding],
	registry ingress.Registry,
	transport http.RoundTripper,
	domain string,
	logger *slog.Logger,
) *taskHandler {
	h := &taskHandler{
		resolver: resolver,
		vms:      vms,
		kinds:    kinds,
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
			// one. The node is asked for its own route; what the task is
			// asked for rides inside it.
			r.Out.URL.Path = upstream.Path
			r.Out.URL.RawQuery = r.In.URL.RawQuery

			// the task is addressed by the name the client used, not by
			// the node it happens to sit on.
			r.Out.Host = r.In.Host

			r.SetXForwarded()
		},
		ErrorHandler: func(rw http.ResponseWriter, r *http.Request, err error) {
			// the node took the request and nothing came back, which for a
			// task that has only just started usually means it is still
			// coming up. Whoever is looking at it is served a page that comes
			// back on its own; anything else is told plainly.
			h.logger.Error("could not reach the node holding a task", "error", err)
			writeStarting(rw, r)
		},
	}

	return h
}

// targetKey carries the resolved upstream from ServeHTTP to the rewrite, which
// is the only hook a ReverseProxy gives for a per-request target.
type targetKey struct{}

func (h *taskHandler) ServeHTTP(rw http.ResponseWriter, r *http.Request) {
	slug, taskPort, ok := h.parseHost(r.Host)
	if !ok {
		http.Error(rw, "unknown task", http.StatusNotFound)

		return
	}

	nodeName, route, refused, err := h.locate(r.Context(), slug, taskPort)
	switch {
	case err != nil:
		infraTrace.RecordError(trace.SpanFromContext(r.Context()), err)
		rw.WriteHeader(http.StatusInternalServerError)

		return
	case refused != nil:
		http.Error(rw, refused.message, refused.status)

		return
	}

	// only the node holding it can reach a VM or a task, and only a node that
	// is connected can be asked to.
	connected, err := h.registry.Exists(r.Context(), nodeName)
	if err != nil {
		infraTrace.RecordError(trace.SpanFromContext(r.Context()), err)
		rw.WriteHeader(http.StatusInternalServerError)

		return
	}

	if !connected {
		http.Error(rw, "the node holding this task is not connected", http.StatusServiceUnavailable)

		return
	}

	target := &url.URL{
		Scheme: "http",
		Host:   nodeName,
		Path: strings.Join([]string{
			route,
			url.PathEscape(slug),
			strconv.FormatUint(uint64(taskPort), 10),
			strings.TrimPrefix(r.URL.Path, "/"),
		}, "/"),
	}

	h.proxy.ServeHTTP(rw, r.WithContext(context.WithValue(r.Context(), targetKey{}, target)))
}

// unavailable is why what a slug names cannot be reached, and the status that
// says so.
type unavailable struct {
	status  int
	message string
}

// unknown is a slug that names nothing the ingress serves. A VM that lets
// nothing in, and a port a VM does not expose, are answered the same way:
// what is not served is not there, as far as anybody asking is concerned.
var unknown = &unavailable{status: http.StatusNotFound, message: "unknown task"}

// The routes a node serves a VM's ports and a task's on. A node finds what a
// slug names among every instance it holds, whichever route it came down.
const (
	vmPortsRoute   = "/vms"
	taskPortsRoute = "/tasks"
)

// locate is the node holding what a slug names, a VM, looked for first, a
// task, or a resource of a kind with endpoints, and the route the node serves
// its ports on.
func (h *taskHandler) locate(ctx context.Context, slug string, requested port.Port) (string, string, *unavailable, error) {
	if h.vms != nil {
		v, err := h.vms.GetOneBySlug(ctx, slug)

		switch {
		case err == nil:
			nodeName, refused := vmNode(&v, requested)

			return nodeName, vmPortsRoute, refused, nil
		case !errors.Is(err, domain.ErrNotExists):
			return "", "", nil, err
		}
	}

	t, err := h.resolver.GetOneBySlug(ctx, slug)
	switch {
	case errors.Is(err, domain.ErrNotExists):
		return h.locateKind(ctx, slug, requested)
	case err != nil:
		return "", "", nil, err
	}

	if t.CurrentState != task.Running {
		return "", "", &unavailable{status: http.StatusServiceUnavailable, message: "the task is not running"}, nil
	}

	if len(t.NodeName) == 0 {
		return "", "", &unavailable{status: http.StatusServiceUnavailable, message: "the task has not been scheduled yet"}, nil
	}

	return t.NodeName, taskPortsRoute, nil, nil
}

// locateKind is the node holding the resource a slug names among the kinds
// with endpoints, asked in the order they were registered, and the route the
// node serves the kind's ports on: under the kind's plural. A kind that has
// nothing by the slug passes it to the next.
func (h *taskHandler) locateKind(ctx context.Context, slug string, requested port.Port) (string, string, *unavailable, error) {
	for _, binding := range h.kinds.All() {
		d := binding.Descriptor()
		if !d.Endpoints {
			continue
		}

		location, err := binding.BySlug(ctx, slug)

		switch {
		case errors.Is(err, domain.ErrNotExists):
			continue
		case errors.Is(err, kind.ErrUnreachable):
			return "", "", &unavailable{status: http.StatusServiceUnavailable, message: err.Error()}, nil
		case err != nil:
			return "", "", nil, err
		}

		nodeName, refused := locatedNode(d.Name, location, requested)

		return nodeName, "/" + d.Plural, refused, nil
	}

	return "", "", unknown, nil
}

// locatedNode is the node holding a resource whose port is asked for, when
// the resource lets the ingress in to that port and is on a node to be
// reached. With no port named, its node answers on the lowest one it exposes.
func locatedNode(kindName string, location kind.Location, requested port.Port) (string, *unavailable) {
	if len(location.Ports) == 0 {
		return "", unknown
	}

	if requested != 0 && !slices.Contains(location.Ports, requested) {
		return "", unknown
	}

	if len(location.Node) == 0 {
		return "", &unavailable{status: http.StatusServiceUnavailable, message: "the " + kindName + " has not been scheduled yet"}
	}

	return location.Node, nil
}

// vmNode is the node holding a VM whose port is asked for, when the VM lets the
// ingress in to that port and is running somewhere to be reached. With no port
// named, its node answers on the lowest one it exposes.
func vmNode(v *vm.VM, requested port.Port) (string, *unavailable) {
	if v.Network.Ingress != vm.AccessAllow || len(v.Ports) == 0 {
		return "", unknown
	}

	if requested != 0 && !slices.Contains(v.Ports, requested) {
		return "", unknown
	}

	if v.CurrentState != vm.Running {
		return "", &unavailable{status: http.StatusServiceUnavailable, message: "the vm is not running"}
	}

	if len(v.NodeName) == 0 {
		return "", &unavailable{status: http.StatusServiceUnavailable, message: "the vm has not been scheduled yet"}
	}

	return v.NodeName, nil
}

// parseHost takes the task's slug, and optionally the port it names, out
// of a hostname. A hostname outside the ingress domain names no task.
//
// The slug's own random suffix is letters only, so a trailing group of digits
// can only ever be a port.
func (h *taskHandler) parseHost(host string) (string, port.Port, bool) {
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

// startingSeconds is how long a page that is waiting for a task waits
// before asking again.
const startingSeconds = 2

// startingPage is what a browser is shown while a task is not answering
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

// writeStarting answers a request for a task that is not answering yet.
// A browser is given the waiting page, which asks again on its own; anything
// else — a fetch, a health check, a command line — is given the bare status it
// can act on.
// starting is what the waiting page says, in the language the browser asked
// for. It knows the two the site is written in and falls back to English,
// which is what the workload itself speaks.
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
		http.Error(rw, "the task is not answering", http.StatusBadGateway)

		return
	}

	lang, dir, text := starting(r.Header.Get("Accept-Language"))

	rw.Header().Set("Content-Type", "text/html; charset=utf-8")
	rw.Header().Set("Vary", "Accept-Language")
	rw.WriteHeader(http.StatusBadGateway)
	_, _ = fmt.Fprintf(rw, startingPage, lang, dir, text)
}
