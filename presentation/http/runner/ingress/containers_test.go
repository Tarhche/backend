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

	"github.com/khanzadimahdi/testproject/domain"
	"github.com/khanzadimahdi/testproject/domain/runner/port"
	"github.com/khanzadimahdi/testproject/domain/runner/task"
)

const testDomain = "runner.localhost"

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

// node is a worker standing in for the far end of a tunnel: it answers the
// route the ingress sends a container's traffic down, and records what it was
// asked for.
type node struct {
	server *httptest.Server

	slug string
	port string
	path string
	host string
}

func newNode(t *testing.T, handler http.Handler) *node {
	t.Helper()

	n := &node{}

	mux := http.NewServeMux()
	mux.Handle("/containers/{slug}/{port}/{path...}", http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
		n.slug = r.PathValue("slug")
		n.port = r.PathValue("port")
		n.path = "/" + r.PathValue("path")
		n.host = r.Host

		handler.ServeHTTP(rw, r)
	}))

	n.server = httptest.NewServer(mux)
	t.Cleanup(n.server.Close)

	return n
}

// ingressFor builds the handler the way the provider does, with a transport
// that stands for the tunnel: the address names a node, and what comes back is
// a connection to the one standing in for it.
func ingressFor(t *testing.T, resolver Resolver, nodes map[string]*node) *containerHandler {
	t.Helper()

	connected := make(connectedRunners, len(nodes))
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

	return NewContainerHandler(resolver, connected, transport, testDomain, slog.New(slog.DiscardHandler))
}

func held(slug string, nodeName string) task.Task {
	return task.Task{Slug: slug, State: task.Running, NodeName: nodeName}
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
		{name: "another domain names no container", host: "nginx-xkfqz.example.com", ok: false},
		{name: "the bare domain names no container", host: testDomain, ok: false},
		{name: "a deeper name is not one of ours", host: "a.nginx-xkfqz." + testDomain, ok: false},
		{name: "nothing at all", host: "", ok: false},
	}

	for _, tt := range testcases {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			slug, containerPort, ok := h.parseHost(tt.host)

			assert.Equal(t, tt.ok, ok)
			if !tt.ok {
				return
			}

			assert.Equal(t, tt.slug, slug)
			assert.Equal(t, tt.port, containerPort)
		})
	}
}

func TestContainerHandler(t *testing.T) {
	t.Run("the request goes to the node holding the container", func(t *testing.T) {
		n := newNode(t, http.HandlerFunc(func(rw http.ResponseWriter, _ *http.Request) {
			io.WriteString(rw, "answered by the container")
		}))

		resolver := &fakeResolver{tasks: map[string]task.Task{
			"nginx-xkfqz": held("nginx-xkfqz", "runner-worker-02"),
		}}

		rw := httptest.NewRecorder()
		request := httptest.NewRequest(http.MethodGet, "/some/path?a=1", nil)
		request.Host = "nginx-xkfqz." + testDomain

		ingressFor(t, resolver, map[string]*node{"runner-worker-02": n}).ServeHTTP(rw, request)

		assert.Equal(t, http.StatusOK, rw.Code)
		assert.Equal(t, "answered by the container", rw.Body.String())
		assert.Equal(t, "nginx-xkfqz", n.slug)
		assert.Equal(t, "/some/path", n.path)
		assert.Equal(t, "nginx-xkfqz."+testDomain, n.host, "the container is addressed by the name the client used")
	})

	t.Run("a bare hostname asks the node for no port in particular", func(t *testing.T) {
		n := newNode(t, http.NotFoundHandler())

		resolver := &fakeResolver{tasks: map[string]task.Task{
			"nginx-xkfqz": held("nginx-xkfqz", "runner-worker-01"),
		}}

		rw := httptest.NewRecorder()
		request := httptest.NewRequest(http.MethodGet, "/", nil)
		request.Host = "nginx-xkfqz." + testDomain

		ingressFor(t, resolver, map[string]*node{"runner-worker-01": n}).ServeHTTP(rw, request)

		assert.Equal(t, "0", n.port, "the node picks the lowest one it finds")
	})

	t.Run("a named port is passed on to the node", func(t *testing.T) {
		n := newNode(t, http.NotFoundHandler())

		resolver := &fakeResolver{tasks: map[string]task.Task{
			"nginx-xkfqz": held("nginx-xkfqz", "runner-worker-01"),
		}}

		rw := httptest.NewRecorder()
		request := httptest.NewRequest(http.MethodGet, "/", nil)
		request.Host = "nginx-xkfqz-8080." + testDomain

		ingressFor(t, resolver, map[string]*node{"runner-worker-01": n}).ServeHTTP(rw, request)

		assert.Equal(t, "8080", n.port)
	})

	t.Run("carries a websocket upgrade through to the container", func(t *testing.T) {
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
			"app-xkfqz": held("app-xkfqz", "runner-worker-01"),
		}}

		front := httptest.NewServer(ingressFor(t, resolver, map[string]*node{"runner-worker-01": n}))
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

	t.Run("an unknown container is not found", func(t *testing.T) {
		rw := httptest.NewRecorder()
		request := httptest.NewRequest(http.MethodGet, "/", nil)
		request.Host = "nobody-xkfqz." + testDomain

		ingressFor(t, &fakeResolver{}, nil).ServeHTTP(rw, request)

		assert.Equal(t, http.StatusNotFound, rw.Code)
	})

	t.Run("a hostname outside the domain names no container", func(t *testing.T) {
		rw := httptest.NewRecorder()
		request := httptest.NewRequest(http.MethodGet, "/", nil)
		request.Host = "nginx-xkfqz.example.com"

		ingressFor(t, &fakeResolver{}, nil).ServeHTTP(rw, request)

		assert.Equal(t, http.StatusNotFound, rw.Code)
	})

	t.Run("a container that is not running is unavailable", func(t *testing.T) {
		resolver := &fakeResolver{tasks: map[string]task.Task{
			"nginx-xkfqz": {Slug: "nginx-xkfqz", State: task.Stopped, NodeName: "runner-worker-01"},
		}}

		rw := httptest.NewRecorder()
		request := httptest.NewRequest(http.MethodGet, "/", nil)
		request.Host = "nginx-xkfqz." + testDomain

		ingressFor(t, resolver, nil).ServeHTTP(rw, request)

		assert.Equal(t, http.StatusServiceUnavailable, rw.Code)
		assert.Contains(t, rw.Body.String(), "not running")
	})

	t.Run("a container that has not been scheduled is unavailable", func(t *testing.T) {
		resolver := &fakeResolver{tasks: map[string]task.Task{
			"nginx-xkfqz": {Slug: "nginx-xkfqz", State: task.Running},
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
			"nginx-xkfqz": held("nginx-xkfqz", "runner-worker-09"),
		}}

		rw := httptest.NewRecorder()
		request := httptest.NewRequest(http.MethodGet, "/", nil)
		request.Host = "nginx-xkfqz." + testDomain

		// the node holding it is not one of the connected ones
		ingressFor(t, resolver, map[string]*node{"runner-worker-01": newNode(t, http.NotFoundHandler())}).ServeHTTP(rw, request)

		assert.Equal(t, http.StatusServiceUnavailable, rw.Code)
		assert.Contains(t, rw.Body.String(), "not connected")
	})
}
