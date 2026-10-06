package ingress

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"slices"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/khanzadimahdi/testproject/domain"
	"github.com/khanzadimahdi/testproject/domain/workload/kind"
	"github.com/khanzadimahdi/testproject/domain/workload/port"
	"github.com/khanzadimahdi/testproject/domain/workload/task"
	"github.com/khanzadimahdi/testproject/domain/workload/vm"
)

// Lamps and kettles are the kinds these tests find: a lamp has a terminal
// and ports, a kettle ports alone, and a bulb a terminal alone. They are
// registered through the bindings every kind is, so what is tested is the
// ingress finding a kind's resources as it does once one is registered.

const (
	unlit kind.State = "unlit"
	lit   kind.State = "lit"
)

// kindNamed is a kind of that name, with endpoints or not, and with a
// terminal or not.
func kindNamed(name string, endpoints bool, terminal bool) kind.Descriptor {
	d := kind.Descriptor{
		Name:      name,
		Plural:    name + "s",
		StateBy:   kind.OnNode,
		Endpoints: endpoints,
		Machine: kind.Machine{
			Initial: unlit,
			States:  []kind.State{unlit, lit, kind.Failed, kind.Deleted},
			Transitions: []kind.Transition{
				{From: unlit, On: kind.OnAction("light"), To: lit},
				{From: kind.Any, On: kind.OnObserved(kind.Failed), To: kind.Failed},
				{From: kind.Any, On: kind.OnAction("delete"), To: kind.Deleted},
			},
			Terminal: []kind.State{unlit, kind.Failed, kind.Deleted},
		},
		Actions: []kind.Action{
			{Name: "light", Runs: kind.OnNode, Mode: kind.ModeCommand, AllowedIn: []kind.State{unlit}, Desires: lit, Permission: "manage", Payload: kind.NoPayload},
			{Name: "delete", Runs: kind.OnNode, Mode: kind.ModeCommand, Desires: kind.Deleted, Permission: "delete", Payload: kind.NoPayload},
			{Name: "state", Runs: kind.OnNode, Mode: kind.ModeQuery, Permission: "show", Payload: kind.NoPayload},
		},
	}

	if terminal {
		d.Actions = append(d.Actions, kind.Action{Name: "attach", Runs: kind.OnNode, Mode: kind.ModeStream, AllowedIn: []kind.State{lit}, Permission: "attach", Payload: kind.NoPayload})
	}

	return d
}

// found is where the ingress strategy says something is, or why it cannot.
type found struct {
	location kind.Location
	err      error
}

// locating is a kind's ingress strategy: what it finds by uuid and by slug,
// and what it was asked.
type locating struct {
	name   string
	byUUID map[string]found
	bySlug map[string]found

	lock  sync.Mutex
	asked []string
}

var _ kind.Ingress = &locating{}

func (l *locating) ByUUID(_ context.Context, uuid string) (kind.Location, error) {
	return l.find(l.byUUID, uuid)
}

func (l *locating) BySlug(_ context.Context, slug string) (kind.Location, error) {
	return l.find(l.bySlug, slug)
}

func (l *locating) find(where map[string]found, key string) (kind.Location, error) {
	l.lock.Lock()
	l.asked = append(l.asked, key)
	l.lock.Unlock()

	if at, ok := where[key]; ok {
		return at.location, at.err
	}

	return kind.Location{}, fmt.Errorf("%w: no %s %q", domain.ErrNotExists, l.name, key)
}

func (l *locating) wasAsked() []string {
	l.lock.Lock()
	defer l.lock.Unlock()

	return slices.Clone(l.asked)
}

// finding are kinds the ingress finds the resources of, registered in the
// order given.
func finding(t *testing.T, kinds ...kind.IngressBinding) *kind.Registry[kind.IngressBinding] {
	t.Helper()

	registry := kind.NewRegistry[kind.IngressBinding]()
	for _, binding := range kinds {
		require.NoError(t, registry.Register(binding))
	}

	return registry
}

// lampAt is a lamp found at uuid and slug, on node, letting the ingress in to
// ports.
func lampAt(node string, ports ...port.Port) kind.Location {
	return kind.Location{UUID: "lamp-uuid", Node: node, Ports: ports}
}

func TestTaskHandler_Kinds(t *testing.T) {
	t.Run("a kind's port is reached through the node holding it, on the kind's own route", func(t *testing.T) {
		n := newNode(t, http.HandlerFunc(func(rw http.ResponseWriter, _ *http.Request) {
			_, _ = io.WriteString(rw, "answered by the lamp")
		}))

		lamps := &locating{name: "lamp", bySlug: map[string]found{"desk-xkfqz": {location: lampAt("workload-orchestrator-02", 80, 8080)}}}

		rw := httptest.NewRecorder()
		request := httptest.NewRequest(http.MethodGet, "/index.html?a=1", nil)
		request.Host = "desk-xkfqz-8080." + testDomain

		ingressWithKinds(t, &fakeResolver{}, &fakeVMResolver{}, finding(t, kind.BindIngress(kindNamed("lamp", true, true), lamps)), map[string]*node{"workload-orchestrator-02": n}).ServeHTTP(rw, request)

		assert.Equal(t, http.StatusOK, rw.Code)
		assert.Equal(t, "answered by the lamp", rw.Body.String())
		assert.Equal(t, "lamps", n.route, "a kind's ports are asked of the node's route for that kind")
		assert.Equal(t, "desk-xkfqz", n.slug)
		assert.Equal(t, "8080", n.port)
		assert.Equal(t, "/index.html", n.path)
		assert.Equal(t, "desk-xkfqz-8080."+testDomain, n.host, "it is addressed by the name the client used")
	})

	t.Run("a bare hostname asks the node for no port in particular", func(t *testing.T) {
		n := newNode(t, http.NotFoundHandler())

		lamps := &locating{name: "lamp", bySlug: map[string]found{"desk-xkfqz": {location: lampAt("workload-orchestrator-01", 80)}}}

		rw := httptest.NewRecorder()
		request := httptest.NewRequest(http.MethodGet, "/", nil)
		request.Host = "desk-xkfqz." + testDomain

		ingressWithKinds(t, &fakeResolver{}, &fakeVMResolver{}, finding(t, kind.BindIngress(kindNamed("lamp", true, true), lamps)), map[string]*node{"workload-orchestrator-01": n}).ServeHTTP(rw, request)

		assert.Equal(t, "lamps", n.route)
		assert.Equal(t, "0", n.port, "the node picks the lowest one the lamp exposes")
	})

	t.Run("today's VMs and tasks are looked for first, and a kind is not asked", func(t *testing.T) {
		for name, tt := range map[string]struct {
			tasks map[string]task.Task
			vms   map[string]vm.VM
			route string
		}{
			"a vm": {
				vms:   map[string]vm.VM{"same-xkfqz": exposing("same-xkfqz", "workload-orchestrator-01", 80)},
				route: "vms",
			},
			"a task": {
				tasks: map[string]task.Task{"same-xkfqz": held("same-xkfqz", "workload-orchestrator-01")},
				route: "tasks",
			},
		} {
			t.Run(name, func(t *testing.T) {
				n := newNode(t, http.NotFoundHandler())

				lamps := &locating{name: "lamp", bySlug: map[string]found{"same-xkfqz": {location: lampAt("workload-orchestrator-01", 80)}}}

				rw := httptest.NewRecorder()
				request := httptest.NewRequest(http.MethodGet, "/", nil)
				request.Host = "same-xkfqz." + testDomain

				ingressWithKinds(t, &fakeResolver{tasks: tt.tasks}, &fakeVMResolver{vms: tt.vms}, finding(t, kind.BindIngress(kindNamed("lamp", true, true), lamps)), map[string]*node{"workload-orchestrator-01": n}).ServeHTTP(rw, request)

				assert.Equal(t, tt.route, n.route)
				assert.Empty(t, lamps.wasAsked())
			})
		}
	})

	t.Run("kinds are asked in the order they were registered, past those with nothing by the slug", func(t *testing.T) {
		n := newNode(t, http.NotFoundHandler())

		kettles := &locating{name: "kettle"}
		lamps := &locating{name: "lamp", bySlug: map[string]found{"desk-xkfqz": {location: lampAt("workload-orchestrator-01", 80)}}}

		rw := httptest.NewRecorder()
		request := httptest.NewRequest(http.MethodGet, "/", nil)
		request.Host = "desk-xkfqz." + testDomain

		kinds := finding(t, kind.BindIngress(kindNamed("kettle", true, false), kettles), kind.BindIngress(kindNamed("lamp", true, true), lamps))

		ingressWithKinds(t, &fakeResolver{}, &fakeVMResolver{}, kinds, map[string]*node{"workload-orchestrator-01": n}).ServeHTTP(rw, request)

		assert.Equal(t, "lamps", n.route)
		assert.Equal(t, []string{"desk-xkfqz"}, kettles.wasAsked(), "the kettles were asked first")
		assert.Equal(t, []string{"desk-xkfqz"}, lamps.wasAsked())
	})

	t.Run("a kind without endpoints is not asked for a slug at all", func(t *testing.T) {
		bulbs := &locating{name: "bulb", bySlug: map[string]found{"desk-xkfqz": {location: lampAt("workload-orchestrator-01", 80)}}}

		rw := httptest.NewRecorder()
		request := httptest.NewRequest(http.MethodGet, "/", nil)
		request.Host = "desk-xkfqz." + testDomain

		ingressWithKinds(t, &fakeResolver{}, &fakeVMResolver{}, finding(t, kind.BindIngress(kindNamed("bulb", false, true), bulbs)), nil).ServeHTTP(rw, request)

		assert.Equal(t, http.StatusNotFound, rw.Code)
		assert.Empty(t, bulbs.wasAsked())
	})

	for name, tt := range map[string]struct {
		found  found
		host   string
		status int
		says   string
	}{
		"a resource that lets nothing in is not there": {
			found:  found{location: lampAt("workload-orchestrator-01")},
			host:   "desk-xkfqz",
			status: http.StatusNotFound,
		},
		"a port it does not let the ingress in to is not there": {
			found:  found{location: lampAt("workload-orchestrator-01", 80)},
			host:   "desk-xkfqz-22",
			status: http.StatusNotFound,
		},
		"one that cannot be reached now is unavailable, and says why": {
			found:  found{err: fmt.Errorf("%w: the lamp is not lit", kind.ErrUnreachable)},
			host:   "desk-xkfqz",
			status: http.StatusServiceUnavailable,
			says:   "the lamp is not lit",
		},
		"one on no node is unavailable": {
			found:  found{location: lampAt("", 80)},
			host:   "desk-xkfqz",
			status: http.StatusServiceUnavailable,
			says:   "the lamp has not been scheduled yet",
		},
		"one whose node is not connected cannot be asked": {
			found:  found{location: lampAt("workload-orchestrator-09", 80)},
			host:   "desk-xkfqz",
			status: http.StatusServiceUnavailable,
			says:   "not connected",
		},
		"a kind that cannot look is an error": {
			found:  found{err: errors.New("the database is gone")},
			host:   "desk-xkfqz",
			status: http.StatusInternalServerError,
		},
	} {
		t.Run(name, func(t *testing.T) {
			lamps := &locating{name: "lamp", bySlug: map[string]found{"desk-xkfqz": tt.found}}

			rw := httptest.NewRecorder()
			request := httptest.NewRequest(http.MethodGet, "/", nil)
			request.Host = tt.host + "." + testDomain

			ingressWithKinds(t, &fakeResolver{}, &fakeVMResolver{}, finding(t, kind.BindIngress(kindNamed("lamp", true, true), lamps)), map[string]*node{"workload-orchestrator-01": newNode(t, http.NotFoundHandler())}).ServeHTTP(rw, request)

			assert.Equal(t, tt.status, rw.Code)
			assert.Contains(t, rw.Body.String(), tt.says)
		})
	}
}

func TestRouteKinds(t *testing.T) {
	logger := slog.New(slog.DiscardHandler)

	t.Run("a terminal in a kind's resource is carried to its node's own route for it", func(t *testing.T) {
		n := newTerminalNode(t)
		connected, transport := tunnelTo(map[string]*terminalNode{"workload-orchestrator-02": n})

		lamps := &locating{name: "lamp", byUUID: map[string]found{"lamp-uuid": {location: lampAt("workload-orchestrator-02")}}}

		mux := http.NewServeMux()
		require.NoError(t, RouteKinds(mux, finding(t, kind.BindIngress(kindNamed("lamp", true, true), lamps)), connected, transport, logger))

		front := httptest.NewServer(mux)
		defer front.Close()

		assert.Equal(t, "echo: ls", talk(t, front, "/lamps/lamp-uuid/attach"))
		assert.Equal(t, "/api/lamps/lamp-uuid/attach", n.path)
	})

	for name, tt := range map[string]struct {
		found  *found
		status int
		says   string
	}{
		"a resource that is not there": {
			status: http.StatusNotFound,
			says:   "no such lamp",
		},
		"one that cannot be reached now": {
			found:  &found{err: fmt.Errorf("%w: the lamp is not lit", kind.ErrUnreachable)},
			status: http.StatusServiceUnavailable,
			says:   "the lamp is not lit",
		},
		"one on no node": {
			found:  &found{location: lampAt("")},
			status: http.StatusServiceUnavailable,
			says:   "the lamp has not been scheduled yet",
		},
		"one whose node is not connected": {
			found:  &found{location: lampAt("workload-orchestrator-09")},
			status: http.StatusServiceUnavailable,
			says:   "not connected",
		},
		"one its kind cannot look for": {
			found:  &found{err: errors.New("the database is gone")},
			status: http.StatusInternalServerError,
		},
	} {
		t.Run("no terminal in "+name, func(t *testing.T) {
			connected, transport := tunnelTo(map[string]*terminalNode{"workload-orchestrator-01": newTerminalNode(t)})

			lamps := &locating{name: "lamp", byUUID: map[string]found{}}
			if tt.found != nil {
				lamps.byUUID["lamp-uuid"] = *tt.found
			}

			mux := http.NewServeMux()
			require.NoError(t, RouteKinds(mux, finding(t, kind.BindIngress(kindNamed("lamp", true, true), lamps)), connected, transport, logger))

			rw := httptest.NewRecorder()
			mux.ServeHTTP(rw, httptest.NewRequest(http.MethodGet, "/lamps/lamp-uuid/attach", nil))

			assert.Equal(t, tt.status, rw.Code)
			assert.Contains(t, rw.Body.String(), tt.says)
		})
	}

	t.Run("a kind with no streams routes none, and no kind routes nothing", func(t *testing.T) {
		connected, transport := tunnelTo(nil)

		for _, kinds := range []*kind.Registry[kind.IngressBinding]{
			finding(t, kind.BindIngress(kindNamed("kettle", true, false), &locating{name: "kettle"})),
			kind.NewRegistry[kind.IngressBinding](),
			nil,
		} {
			mux := http.NewServeMux()
			require.NoError(t, RouteKinds(mux, kinds, connected, transport, logger))

			_, pattern := mux.Handler(httptest.NewRequest(http.MethodGet, "/kettles/kettle-uuid/attach", nil))
			assert.Empty(t, pattern, "nothing is routed")
		}
	})

	t.Run("the VMs' and the tasks' own routes are left as they are", func(t *testing.T) {
		connected, transport := tunnelTo(nil)

		own := http.HandlerFunc(func(rw http.ResponseWriter, _ *http.Request) { rw.WriteHeader(http.StatusTeapot) })

		mux := http.NewServeMux()
		mux.Handle("GET /tasks/{uuid}/attach", own)
		mux.Handle("GET /vms/{uuid}/attach", own)

		require.NoError(t, RouteKinds(mux, finding(t, kind.BindIngress(kindNamed("lamp", true, true), &locating{name: "lamp"})), connected, transport, logger))

		for _, path := range []string{"/tasks/task-uuid/attach", "/vms/vm-uuid/attach"} {
			rw := httptest.NewRecorder()
			mux.ServeHTTP(rw, httptest.NewRequest(http.MethodGet, path, nil))

			assert.Equal(t, http.StatusTeapot, rw.Code, path)
		}

		_, pattern := mux.Handler(httptest.NewRequest(http.MethodGet, "/lamps/lamp-uuid/attach", nil))
		assert.Equal(t, "GET /lamps/{uuid}/attach", pattern)
	})

	t.Run("a kind whose route is taken already is an error, not a panic", func(t *testing.T) {
		connected, transport := tunnelTo(nil)

		mux := http.NewServeMux()
		mux.Handle("GET /lamps/{uuid}/attach", http.NotFoundHandler())

		err := RouteKinds(mux, finding(t, kind.BindIngress(kindNamed("lamp", true, true), &locating{name: "lamp"})), connected, transport, logger)

		assert.ErrorContains(t, err, "cannot be routed")
	})
}
