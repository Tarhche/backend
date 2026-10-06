package ingress

import (
	"context"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gorilla/websocket"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	ingressVMs "github.com/khanzadimahdi/testproject/application/workload/ingress/kinds/vm"
	"github.com/khanzadimahdi/testproject/domain"
	"github.com/khanzadimahdi/testproject/domain/workload/kind"
	vmKind "github.com/khanzadimahdi/testproject/domain/workload/kinds/vm"
	"github.com/khanzadimahdi/testproject/domain/workload/port"
	"github.com/khanzadimahdi/testproject/domain/workload/resource"
	"github.com/khanzadimahdi/testproject/domain/workload/task"
	"github.com/khanzadimahdi/testproject/domain/workload/vm"
	resourcesMemory "github.com/khanzadimahdi/testproject/infrastructure/repository/memory/workload/resources"
)

const testDomain = "workload.localhost"

// fakeResolver stands in for the task repository: which node is holding what.
type fakeResolver struct {
	tasks map[string]task.Task
}

func (r *fakeResolver) GetOneBySlug(_ context.Context, slug string) (task.Task, error) {
	t, ok := r.tasks[slug]
	if !ok {
		return task.Task{}, domain.ErrNotExists
	}

	return t, nil
}

func (r *fakeResolver) GetOne(_ context.Context, uuid string) (task.Task, error) {
	for _, t := range r.tasks {
		if t.UUID == uuid {
			return t, nil
		}
	}

	return task.Task{}, domain.ErrNotExists
}

// node is an orchestrator standing in for the far end of a tunnel: it answers the
// route the ingress sends a task's traffic down, and records what it was
// asked for.
type node struct {
	server *httptest.Server

	// route is which of the node's routes for ports was asked.
	route string

	slug string
	port string
	path string
	host string
}

func newNode(t *testing.T, handler http.Handler) *node {
	t.Helper()

	n := &node{}

	ports := func(route string) http.Handler {
		return http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
			n.route = route
			n.slug = r.PathValue("slug")
			n.port = r.PathValue("port")
			n.path = "/" + r.PathValue("path")
			n.host = r.Host

			handler.ServeHTTP(rw, r)
		})
	}

	mux := http.NewServeMux()
	mux.Handle("/tasks/{slug}/{port}/{path...}", ports("tasks"))
	mux.Handle("/vms/{slug}/{port}/{path...}", ports("vms"))
	mux.Handle("/lamps/{slug}/{port}/{path...}", ports("lamps"))
	mux.Handle("/kettles/{slug}/{port}/{path...}", ports("kettles"))

	n.server = httptest.NewServer(mux)
	t.Cleanup(n.server.Close)

	return n
}

// ingressFor builds the handler the way the provider does, with a transport
// that stands for the tunnel: the address names a node, and what comes back is
// a connection to the one standing in for it.
func ingressFor(t *testing.T, resolver Resolver, nodes map[string]*node) *taskHandler {
	t.Helper()

	return ingressWithKinds(t, resolver, nil, nodes)
}

// ingressWithVMs is ingressFor with VMs to find as well as tasks, as the vm
// kind's ingress strategy finds them in the records kept of them.
func ingressWithVMs(t *testing.T, resolver Resolver, vms []vmKind.VM, nodes map[string]*node) *taskHandler {
	t.Helper()

	return ingressWithKinds(t, resolver, vmsIn(t, vms...), nodes)
}

// ingressWithKinds is ingressFor with the resources of kinds to find as well.
func ingressWithKinds(t *testing.T, resolver Resolver, kinds *kind.Registry[kind.IngressBinding], nodes map[string]*node) *taskHandler {
	t.Helper()

	connected := make(connectedWorkloads, len(nodes))
	for name := range nodes {
		connected[name] = name
	}

	transport := &http.Transport{
		DialContext: func(ctx context.Context, network string, address string) (net.Conn, error) {
			name, _, _ := net.SplitHostPort(address)

			n, ok := nodes[name]
			if !ok {
				return nil, domain.ErrNotExists
			}

			return (&net.Dialer{}).DialContext(ctx, "tcp", strings.TrimPrefix(n.server.URL, "http://"))
		},
	}

	return NewTaskHandler(resolver, kinds, connected, transport, testDomain, slog.New(slog.DiscardHandler))
}

// vmsIn is the vm kind registered as the ingress registers it, finding vms
// in the records the control plane keeps of them.
func vmsIn(t *testing.T, vms ...vmKind.VM) *kind.Registry[kind.IngressBinding] {
	t.Helper()

	records := make([]resource.Record, len(vms))
	for i := range vms {
		raw, err := kind.Encode(vms[i])
		require.NoError(t, err)

		records[i] = resource.Record{Raw: raw}
	}

	return finding(t, kind.BindIngress(vmKind.Descriptor(), ingressVMs.New(resourcesMemory.NewRepository(records...))))
}

func held(slug string, nodeName string) task.Task {
	return task.Task{Slug: slug, CurrentState: task.Running, NodeName: nodeName}
}

func TestParseHost(t *testing.T) {
	t.Parallel()

	h := ingressFor(t, &fakeResolver{}, nil)

	testcases := []struct {
		name string
		host string
		slug string
		port port.Port
		ok   bool
	}{
		{name: "a bare slug names no port", host: "nginx-xkfqz." + testDomain, slug: "nginx-xkfqz", ok: true},
		{name: "a port comes off the end", host: "nginx-xkfqz-8080." + testDomain, slug: "nginx-xkfqz", port: 8080, ok: true},
		{name: "the browser's port is ignored", host: "nginx-xkfqz." + testDomain + ":8021", slug: "nginx-xkfqz", ok: true},
		{name: "a port with the browser's port too", host: "nginx-xkfqz-443." + testDomain + ":8021", slug: "nginx-xkfqz", port: 443, ok: true},
		{name: "case is folded", host: "NGINX-XKFQZ." + strings.ToUpper(testDomain), slug: "nginx-xkfqz", ok: true},
		{name: "a name ending in letters is not a port", host: "my-web-server." + testDomain, slug: "my-web-server", ok: true},
		{name: "a hyphenated name with a port", host: "my-web-server-8080." + testDomain, slug: "my-web-server", port: 8080, ok: true},
		{name: "port zero is not a port", host: "nginx-xkfqz-0." + testDomain, slug: "nginx-xkfqz-0", ok: true},
		{name: "a port beyond the range is part of the name", host: "nginx-99999." + testDomain, slug: "nginx-99999", ok: true},
		{name: "another domain names no task", host: "nginx-xkfqz.example.com", ok: false},
		{name: "the bare domain names no task", host: testDomain, ok: false},
		{name: "a deeper name is not one of ours", host: "a.nginx-xkfqz." + testDomain, ok: false},
		{name: "nothing at all", host: "", ok: false},
	}

	for _, tt := range testcases {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			slug, taskPort, ok := h.parseHost(tt.host)

			assert.Equal(t, tt.ok, ok)
			if !tt.ok {
				return
			}

			assert.Equal(t, tt.slug, slug)
			assert.Equal(t, tt.port, taskPort)
		})
	}
}

func TestTaskHandler(t *testing.T) {
	t.Run("the request goes to the node holding the task", func(t *testing.T) {
		n := newNode(t, http.HandlerFunc(func(rw http.ResponseWriter, _ *http.Request) {
			io.WriteString(rw, "answered by the task")
		}))

		resolver := &fakeResolver{tasks: map[string]task.Task{
			"nginx-xkfqz": held("nginx-xkfqz", "workload-orchestrator-02"),
		}}

		rw := httptest.NewRecorder()
		request := httptest.NewRequest(http.MethodGet, "/some/path?a=1", nil)
		request.Host = "nginx-xkfqz." + testDomain

		ingressFor(t, resolver, map[string]*node{"workload-orchestrator-02": n}).ServeHTTP(rw, request)

		assert.Equal(t, http.StatusOK, rw.Code)
		assert.Equal(t, "answered by the task", rw.Body.String())
		assert.Equal(t, "tasks", n.route)
		assert.Equal(t, "nginx-xkfqz", n.slug)
		assert.Equal(t, "/some/path", n.path)
		assert.Equal(t, "nginx-xkfqz."+testDomain, n.host, "the task is addressed by the name the client used")
	})

	t.Run("a bare hostname asks the node for no port in particular", func(t *testing.T) {
		n := newNode(t, http.NotFoundHandler())

		resolver := &fakeResolver{tasks: map[string]task.Task{
			"nginx-xkfqz": held("nginx-xkfqz", "workload-orchestrator-01"),
		}}

		rw := httptest.NewRecorder()
		request := httptest.NewRequest(http.MethodGet, "/", nil)
		request.Host = "nginx-xkfqz." + testDomain

		ingressFor(t, resolver, map[string]*node{"workload-orchestrator-01": n}).ServeHTTP(rw, request)

		assert.Equal(t, "0", n.port, "the node picks the lowest one it finds")
	})

	t.Run("a named port is passed on to the node", func(t *testing.T) {
		n := newNode(t, http.NotFoundHandler())

		resolver := &fakeResolver{tasks: map[string]task.Task{
			"nginx-xkfqz": held("nginx-xkfqz", "workload-orchestrator-01"),
		}}

		rw := httptest.NewRecorder()
		request := httptest.NewRequest(http.MethodGet, "/", nil)
		request.Host = "nginx-xkfqz-8080." + testDomain

		ingressFor(t, resolver, map[string]*node{"workload-orchestrator-01": n}).ServeHTTP(rw, request)

		assert.Equal(t, "8080", n.port)
	})

	t.Run("carries a websocket upgrade through to the task", func(t *testing.T) {
		upgrader := websocket.Upgrader{}
		n := newNode(t, http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
			conn, err := upgrader.Upgrade(rw, r, nil)
			if err != nil {
				return
			}
			defer conn.Close()

			_, message, err := conn.ReadMessage()
			if err != nil {
				return
			}

			_ = conn.WriteMessage(websocket.TextMessage, append([]byte("echo: "), message...))
		}))

		resolver := &fakeResolver{tasks: map[string]task.Task{
			"app-xkfqz": held("app-xkfqz", "workload-orchestrator-01"),
		}}

		front := httptest.NewServer(ingressFor(t, resolver, map[string]*node{"workload-orchestrator-01": n}))
		defer front.Close()

		endpoint := "ws://" + strings.TrimPrefix(front.URL, "http://") + "/ws"

		dialer := websocket.Dialer{}
		conn, _, err := dialer.Dial(endpoint, http.Header{"Host": []string{"app-xkfqz." + testDomain}})
		require.NoError(t, err)
		defer conn.Close()

		require.NoError(t, conn.WriteMessage(websocket.TextMessage, []byte("hello")))

		_, message, err := conn.ReadMessage()
		require.NoError(t, err)
		assert.Equal(t, "echo: hello", string(message))
	})

	t.Run("an unknown task is not found", func(t *testing.T) {
		rw := httptest.NewRecorder()
		request := httptest.NewRequest(http.MethodGet, "/", nil)
		request.Host = "nobody-xkfqz." + testDomain

		ingressFor(t, &fakeResolver{}, nil).ServeHTTP(rw, request)

		assert.Equal(t, http.StatusNotFound, rw.Code)
	})

	t.Run("a hostname outside the domain names no task", func(t *testing.T) {
		rw := httptest.NewRecorder()
		request := httptest.NewRequest(http.MethodGet, "/", nil)
		request.Host = "nginx-xkfqz.example.com"

		ingressFor(t, &fakeResolver{}, nil).ServeHTTP(rw, request)

		assert.Equal(t, http.StatusNotFound, rw.Code)
	})

	t.Run("a task that is not running is unavailable", func(t *testing.T) {
		resolver := &fakeResolver{tasks: map[string]task.Task{
			"nginx-xkfqz": {Slug: "nginx-xkfqz", CurrentState: task.Stopped, NodeName: "workload-orchestrator-01"},
		}}

		rw := httptest.NewRecorder()
		request := httptest.NewRequest(http.MethodGet, "/", nil)
		request.Host = "nginx-xkfqz." + testDomain

		ingressFor(t, resolver, nil).ServeHTTP(rw, request)

		assert.Equal(t, http.StatusServiceUnavailable, rw.Code)
		assert.Contains(t, rw.Body.String(), "not running")
	})

	t.Run("a task that has not been scheduled is unavailable", func(t *testing.T) {
		resolver := &fakeResolver{tasks: map[string]task.Task{
			"nginx-xkfqz": {Slug: "nginx-xkfqz", CurrentState: task.Running},
		}}

		rw := httptest.NewRecorder()
		request := httptest.NewRequest(http.MethodGet, "/", nil)
		request.Host = "nginx-xkfqz." + testDomain

		ingressFor(t, resolver, nil).ServeHTTP(rw, request)

		assert.Equal(t, http.StatusServiceUnavailable, rw.Code)
		assert.Contains(t, rw.Body.String(), "not been scheduled")
	})

	t.Run("a node that is not connected cannot be asked", func(t *testing.T) {
		resolver := &fakeResolver{tasks: map[string]task.Task{
			"nginx-xkfqz": held("nginx-xkfqz", "workload-orchestrator-09"),
		}}

		rw := httptest.NewRecorder()
		request := httptest.NewRequest(http.MethodGet, "/", nil)
		request.Host = "nginx-xkfqz." + testDomain

		// the node holding it is not one of the connected ones
		ingressFor(t, resolver, map[string]*node{"workload-orchestrator-01": newNode(t, http.NotFoundHandler())}).ServeHTTP(rw, request)

		assert.Equal(t, http.StatusServiceUnavailable, rw.Code)
		assert.Contains(t, rw.Body.String(), "not connected")
	})
}

// A task that is not answering yet is usually one that has only just
// started, so whoever is looking at it is given something that comes back on
// its own rather than an error to refresh by hand.
func TestWaitingPage(t *testing.T) {
	// a node that takes the request and cannot reach the task
	unreachable := func(t *testing.T) *node {
		return newNode(t, http.HandlerFunc(func(rw http.ResponseWriter, _ *http.Request) {
			// the node accepts and then goes away mid-answer, which is what a
			// task that is still coming up looks like from here
			conn, _, err := rw.(http.Hijacker).Hijack()
			if err != nil {
				return
			}

			conn.Close()
		}))
	}

	resolver := &fakeResolver{tasks: map[string]task.Task{
		"nginx-xkfqz": held("nginx-xkfqz", "workload-orchestrator-01"),
	}}

	t.Run("a browser gets a page that comes back on its own", func(t *testing.T) {
		n := unreachable(t)

		rw := httptest.NewRecorder()
		request := httptest.NewRequest(http.MethodGet, "/", nil)
		request.Host = "nginx-xkfqz." + testDomain
		request.Header.Set("Accept", "text/html,application/xhtml+xml,*/*;q=0.8")

		ingressFor(t, resolver, map[string]*node{"workload-orchestrator-01": n}).ServeHTTP(rw, request)

		assert.Equal(t, http.StatusBadGateway, rw.Code)
		assert.Equal(t, "2", rw.Header().Get("Retry-After"))
		assert.Contains(t, rw.Header().Get("Content-Type"), "text/html")
		assert.Contains(t, rw.Body.String(), `http-equiv="refresh"`)
		assert.Contains(t, rw.Body.String(), "starting")
	})

	t.Run("the page speaks the language the browser asked for", func(t *testing.T) {
		n := unreachable(t)

		rw := httptest.NewRecorder()
		request := httptest.NewRequest(http.MethodGet, "/", nil)
		request.Host = "nginx-xkfqz." + testDomain
		request.Header.Set("Accept", "text/html")
		request.Header.Set("Accept-Language", "fa-IR,fa;q=0.9,en;q=0.8")

		ingressFor(t, resolver, map[string]*node{"workload-orchestrator-01": n}).ServeHTTP(rw, request)

		assert.Contains(t, rw.Body.String(), `lang="fa" dir="rtl"`)
		assert.Contains(t, rw.Body.String(), "آماده‌سازی")
	})

	t.Run("anything that did not ask for a page is told plainly", func(t *testing.T) {
		n := unreachable(t)

		rw := httptest.NewRecorder()
		request := httptest.NewRequest(http.MethodGet, "/api/health", nil)
		request.Host = "nginx-xkfqz." + testDomain

		ingressFor(t, resolver, map[string]*node{"workload-orchestrator-01": n}).ServeHTTP(rw, request)

		assert.Equal(t, http.StatusBadGateway, rw.Code)
		assert.Equal(t, "2", rw.Header().Get("Retry-After"))
		assert.NotContains(t, rw.Body.String(), "<html", "a client that did not ask for a page is not given one")
	})
}

// exposing is a running VM whose ports are reached through the ingress.
func exposing(slug string, nodeName string, ports ...port.Port) vmKind.VM {
	return vmKind.VM{
		Kind:     vmKind.Name,
		Metadata: kind.Metadata{UUID: "vm-" + slug, Slug: slug, Node: nodeName},
		Spec: vmKind.Spec{
			Flavor:  vmKind.FlavorMachine,
			Ports:   ports,
			Network: vmKind.Network{Ingress: vm.AccessAllow, Egress: vm.AccessAllow},
		},
		Status: vmKind.Status{Status: kind.Status{State: vmKind.Running, Expected: vmKind.Running}},
	}
}

// changed is v as change leaves it.
func changed(v vmKind.VM, change func(v *vmKind.VM)) vmKind.VM {
	change(&v)

	return v
}

func TestTaskHandler_VMs(t *testing.T) {
	t.Run("a vm's port is reached through the node holding it, on the vms' route", func(t *testing.T) {
		n := newNode(t, http.HandlerFunc(func(rw http.ResponseWriter, _ *http.Request) {
			io.WriteString(rw, "answered by the vm")
		}))

		rw := httptest.NewRecorder()
		request := httptest.NewRequest(http.MethodGet, "/index.html?a=1", nil)
		request.Host = "box-xkfqz-8080." + testDomain

		ingressWithVMs(t, &fakeResolver{}, []vmKind.VM{exposing("box-xkfqz", "workload-orchestrator-02", 80, 8080)}, map[string]*node{"workload-orchestrator-02": n}).ServeHTTP(rw, request)

		assert.Equal(t, http.StatusOK, rw.Code)
		assert.Equal(t, "answered by the vm", rw.Body.String())
		assert.Equal(t, "vms", n.route, "a vm's ports are asked of the node's route for them")
		assert.Equal(t, "box-xkfqz", n.slug)
		assert.Equal(t, "8080", n.port)
		assert.Equal(t, "/index.html", n.path)
		assert.Equal(t, "box-xkfqz-8080."+testDomain, n.host)
	})

	t.Run("a bare hostname asks the node for the lowest port the vm exposes", func(t *testing.T) {
		n := newNode(t, http.NotFoundHandler())

		rw := httptest.NewRecorder()
		request := httptest.NewRequest(http.MethodGet, "/", nil)
		request.Host = "box-xkfqz." + testDomain

		ingressWithVMs(t, &fakeResolver{}, []vmKind.VM{exposing("box-xkfqz", "workload-orchestrator-01", 80)}, map[string]*node{"workload-orchestrator-01": n}).ServeHTTP(rw, request)

		assert.Equal(t, "box-xkfqz", n.slug, "the vm's node was asked")
		assert.Equal(t, "0", n.port, "the node picks the lowest port the vm exposes")
	})

	for name, tt := range map[string]struct {
		vm     vmKind.VM
		host   string
		status int
		says   string
	}{
		"a vm that lets nothing in is not there": {
			vm:     changed(exposing("box-xkfqz", "workload-orchestrator-01", 80), func(v *vmKind.VM) { v.Spec.Network.Ingress = vm.AccessDeny }),
			host:   "box-xkfqz-80",
			status: http.StatusNotFound,
		},
		"nor is one that lets nothing in and is not running": {
			vm: changed(exposing("box-xkfqz", "workload-orchestrator-01", 80), func(v *vmKind.VM) {
				v.Spec.Network.Ingress = vm.AccessDeny
				v.Status.State = vmKind.Stopped
			}),
			host:   "box-xkfqz",
			status: http.StatusNotFound,
		},
		"a port a vm does not expose is not there": {
			vm:     exposing("box-xkfqz", "workload-orchestrator-01", 80),
			host:   "box-xkfqz-22",
			status: http.StatusNotFound,
		},
		"nor is one a vm that is not running does not expose": {
			vm:     changed(exposing("box-xkfqz", "workload-orchestrator-01", 80), func(v *vmKind.VM) { v.Status.State = vmKind.Stopped }),
			host:   "box-xkfqz-22",
			status: http.StatusNotFound,
		},
		"a vm that exposes nothing is not there": {
			vm:     exposing("box-xkfqz", "workload-orchestrator-01"),
			host:   "box-xkfqz",
			status: http.StatusNotFound,
		},
		"a vm that is not running is unavailable": {
			vm:     changed(exposing("box-xkfqz", "workload-orchestrator-01", 80), func(v *vmKind.VM) { v.Status.State = vmKind.Stopped }),
			host:   "box-xkfqz-80",
			status: http.StatusServiceUnavailable,
			says:   "the vm is not running",
		},
		"a vm on no node is unavailable": {
			vm:     exposing("box-xkfqz", "", 80),
			host:   "box-xkfqz",
			status: http.StatusServiceUnavailable,
			says:   "not been scheduled",
		},
		"a vm whose node is not connected cannot be asked": {
			vm:     exposing("box-xkfqz", "workload-orchestrator-09", 80),
			host:   "box-xkfqz",
			status: http.StatusServiceUnavailable,
			says:   "not connected",
		},
	} {
		t.Run(name, func(t *testing.T) {
			rw := httptest.NewRecorder()
			request := httptest.NewRequest(http.MethodGet, "/", nil)
			request.Host = tt.host + "." + testDomain

			ingressWithVMs(t, &fakeResolver{}, []vmKind.VM{tt.vm}, map[string]*node{"workload-orchestrator-01": newNode(t, http.NotFoundHandler())}).ServeHTTP(rw, request)

			assert.Equal(t, tt.status, rw.Code)
			assert.Contains(t, rw.Body.String(), tt.says)
		})
	}
}
