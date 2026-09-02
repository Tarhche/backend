package ingress

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
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

// fakeResolver stands in for the task repository.
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

// runningContainer builds a task whose endpoints point at the given upstreams,
// keyed by container port.
func runningContainer(slug string, upstreams map[port.Port]*httptest.Server) task.Task {
	endpoints := make([]task.Endpoint, 0, len(upstreams))

	for containerPort, server := range upstreams {
		parsed, err := url.Parse(server.URL)
		if err != nil {
			panic(err)
		}

		hostPort, err := strconv.ParseUint(parsed.Port(), 10, 16)
		if err != nil {
			panic(err)
		}

		endpoints = append(endpoints, task.Endpoint{
			ContainerPort: containerPort,
			Host:          parsed.Hostname(),
			HostPort:      port.Port(hostPort),
		})
	}

	return task.Task{Slug: slug, State: task.Running, Endpoints: endpoints}
}

func TestParseHost(t *testing.T) {
	t.Parallel()

	h := NewHandler(&fakeResolver{}, testDomain)

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

func TestHandler(t *testing.T) {
	t.Parallel()

	t.Run("proxies a bare slug to the lowest exposed port", func(t *testing.T) {
		t.Parallel()

		low := httptest.NewServer(http.HandlerFunc(func(rw http.ResponseWriter, _ *http.Request) {
			_, _ = io.WriteString(rw, "port 80")
		}))
		defer low.Close()

		high := httptest.NewServer(http.HandlerFunc(func(rw http.ResponseWriter, _ *http.Request) {
			_, _ = io.WriteString(rw, "port 8080")
		}))
		defer high.Close()

		resolver := &fakeResolver{tasks: map[string]task.Task{
			"nginx-xkfqz": runningContainer("nginx-xkfqz", map[port.Port]*httptest.Server{80: low, 8080: high}),
		}}

		rw := httptest.NewRecorder()
		request := httptest.NewRequest(http.MethodGet, "/", nil)
		request.Host = "nginx-xkfqz." + testDomain

		NewHandler(resolver, testDomain).ServeHTTP(rw, request)

		assert.Equal(t, http.StatusOK, rw.Code)
		assert.Equal(t, "port 80", rw.Body.String())
	})

	t.Run("proxies a named port to that port", func(t *testing.T) {
		t.Parallel()

		low := httptest.NewServer(http.HandlerFunc(func(rw http.ResponseWriter, _ *http.Request) {
			_, _ = io.WriteString(rw, "port 80")
		}))
		defer low.Close()

		high := httptest.NewServer(http.HandlerFunc(func(rw http.ResponseWriter, _ *http.Request) {
			_, _ = io.WriteString(rw, "port 8080")
		}))
		defer high.Close()

		resolver := &fakeResolver{tasks: map[string]task.Task{
			"nginx-xkfqz": runningContainer("nginx-xkfqz", map[port.Port]*httptest.Server{80: low, 8080: high}),
		}}

		rw := httptest.NewRecorder()
		request := httptest.NewRequest(http.MethodGet, "/", nil)
		request.Host = "nginx-xkfqz-8080." + testDomain

		NewHandler(resolver, testDomain).ServeHTTP(rw, request)

		assert.Equal(t, http.StatusOK, rw.Code)
		assert.Equal(t, "port 8080", rw.Body.String())
	})

	t.Run("passes the path, query and the name the client used through", func(t *testing.T) {
		t.Parallel()

		var (
			gotPath  string
			gotQuery string
			gotHost  string
		)

		upstream := httptest.NewServer(http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
			gotPath, gotQuery, gotHost = r.URL.Path, r.URL.RawQuery, r.Host
		}))
		defer upstream.Close()

		resolver := &fakeResolver{tasks: map[string]task.Task{
			"nginx-xkfqz": runningContainer("nginx-xkfqz", map[port.Port]*httptest.Server{80: upstream}),
		}}

		rw := httptest.NewRecorder()
		request := httptest.NewRequest(http.MethodGet, "/some/path?a=1", nil)
		request.Host = "nginx-xkfqz." + testDomain

		NewHandler(resolver, testDomain).ServeHTTP(rw, request)

		assert.Equal(t, "/some/path", gotPath)
		assert.Equal(t, "a=1", gotQuery)
		assert.Equal(t, "nginx-xkfqz."+testDomain, gotHost, "the container is addressed by the name the client used")
	})

	t.Run("carries a websocket upgrade through to the container", func(t *testing.T) {
		t.Parallel()

		upgrader := websocket.Upgrader{}
		upstream := httptest.NewServer(http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
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
		defer upstream.Close()

		resolver := &fakeResolver{tasks: map[string]task.Task{
			"app-xkfqz": runningContainer("app-xkfqz", map[port.Port]*httptest.Server{80: upstream}),
		}}

		ingress := httptest.NewServer(NewHandler(resolver, testDomain))
		defer ingress.Close()

		endpoint := "ws://" + strings.TrimPrefix(ingress.URL, "http://") + "/ws"

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
		t.Parallel()

		rw := httptest.NewRecorder()
		request := httptest.NewRequest(http.MethodGet, "/", nil)
		request.Host = "nobody-xkfqz." + testDomain

		NewHandler(&fakeResolver{}, testDomain).ServeHTTP(rw, request)

		assert.Equal(t, http.StatusNotFound, rw.Code)
	})

	t.Run("a hostname outside the runner domain is not found", func(t *testing.T) {
		t.Parallel()

		upstream := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
		defer upstream.Close()

		resolver := &fakeResolver{tasks: map[string]task.Task{
			"nginx-xkfqz": runningContainer("nginx-xkfqz", map[port.Port]*httptest.Server{80: upstream}),
		}}

		rw := httptest.NewRecorder()
		request := httptest.NewRequest(http.MethodGet, "/", nil)
		request.Host = "nginx-xkfqz.example.com"

		NewHandler(resolver, testDomain).ServeHTTP(rw, request)

		assert.Equal(t, http.StatusNotFound, rw.Code)
	})

	t.Run("a port the container does not expose is not found", func(t *testing.T) {
		t.Parallel()

		upstream := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
		defer upstream.Close()

		resolver := &fakeResolver{tasks: map[string]task.Task{
			"nginx-xkfqz": runningContainer("nginx-xkfqz", map[port.Port]*httptest.Server{80: upstream}),
		}}

		rw := httptest.NewRecorder()
		request := httptest.NewRequest(http.MethodGet, "/", nil)
		request.Host = "nginx-xkfqz-9999." + testDomain

		NewHandler(resolver, testDomain).ServeHTTP(rw, request)

		assert.Equal(t, http.StatusNotFound, rw.Code)
	})

	t.Run("a container that exposes nothing is not found", func(t *testing.T) {
		t.Parallel()

		resolver := &fakeResolver{tasks: map[string]task.Task{
			"nginx-xkfqz": {Slug: "nginx-xkfqz", State: task.Running},
		}}

		rw := httptest.NewRecorder()
		request := httptest.NewRequest(http.MethodGet, "/", nil)
		request.Host = "nginx-xkfqz." + testDomain

		NewHandler(resolver, testDomain).ServeHTTP(rw, request)

		assert.Equal(t, http.StatusNotFound, rw.Code)
	})

	t.Run("a container that is not running is unavailable", func(t *testing.T) {
		t.Parallel()

		upstream := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
		defer upstream.Close()

		stopped := runningContainer("nginx-xkfqz", map[port.Port]*httptest.Server{80: upstream})
		stopped.State = task.Stopped

		resolver := &fakeResolver{tasks: map[string]task.Task{"nginx-xkfqz": stopped}}

		rw := httptest.NewRecorder()
		request := httptest.NewRequest(http.MethodGet, "/", nil)
		request.Host = "nginx-xkfqz." + testDomain

		NewHandler(resolver, testDomain).ServeHTTP(rw, request)

		assert.Equal(t, http.StatusServiceUnavailable, rw.Code)
	})

	t.Run("a container that is not answering is a bad gateway", func(t *testing.T) {
		t.Parallel()

		dead := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
		unreachable := runningContainer("nginx-xkfqz", map[port.Port]*httptest.Server{80: dead})
		dead.Close()

		resolver := &fakeResolver{tasks: map[string]task.Task{"nginx-xkfqz": unreachable}}

		rw := httptest.NewRecorder()
		request := httptest.NewRequest(http.MethodGet, "/", nil)
		request.Host = "nginx-xkfqz." + testDomain

		NewHandler(resolver, testDomain).ServeHTTP(rw, request)

		assert.Equal(t, http.StatusBadGateway, rw.Code)
	})
}
