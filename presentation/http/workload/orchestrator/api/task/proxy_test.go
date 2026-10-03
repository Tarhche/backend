package task

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync"
	"testing"

	"github.com/gorilla/websocket"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	getendpoint "github.com/khanzadimahdi/testproject/application/workload/orchestrator/task/getEndpoint"
	"github.com/khanzadimahdi/testproject/domain/workload/port"
	"github.com/khanzadimahdi/testproject/domain/workload/task"
	"github.com/khanzadimahdi/testproject/infrastructure/repository/mocks/workload/runtime"
)

// dialingRuntime stands for a runtime that reaches its runs itself, as the
// microvm driver does: asked for a run and one of its ports, it connects to
// the server standing in for that run, and remembers what it was asked for.
type dialingRuntime struct {
	runtime.MockRuntime

	upstream string
	err      error

	lock    sync.Mutex
	dialled []string
}

var _ task.Dialer = &dialingRuntime{}

func (r *dialingRuntime) DialContext(ctx context.Context, executionID string, p port.Port) (net.Conn, error) {
	r.lock.Lock()
	r.dialled = append(r.dialled, executionID+"/"+strconv.FormatUint(uint64(p), 10))
	r.lock.Unlock()

	if r.err != nil {
		return nil, r.err
	}

	return (&net.Dialer{}).DialContext(ctx, "tcp", r.upstream)
}

func (r *dialingRuntime) asked() []string {
	r.lock.Lock()
	defer r.lock.Unlock()

	return append([]string(nil), r.dialled...)
}

// holding makes a runtime hold one running task, answering to its slug, and
// nothing else.
func holding(taskManager *runtime.MockRuntime, run task.Execution) {
	taskManager.On("BySlug", mock.Anything, "nginx-xkfqz").Return([]task.Execution{run}, nil)
	taskManager.On("BySlug", mock.Anything, mock.Anything).Return([]task.Execution{}, nil)
}

// serve builds the route the way the provider does, over the given runtime.
func serve(t *testing.T, taskManager task.Runtime, advertiseHost string) *httptest.Server {
	t.Helper()

	mux := http.NewServeMux()
	mux.Handle("/tasks/{slug}/{port}/{path...}", NewProxyHandler(getendpoint.NewUseCase(taskManager, advertiseHost), slog.New(slog.DiscardHandler)))

	server := httptest.NewServer(mux)
	t.Cleanup(server.Close)

	return server
}

// upstream stands for a task's port, saying what it was asked for.
func upstream(t *testing.T, handler http.HandlerFunc) *httptest.Server {
	t.Helper()

	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)

	return server
}

// hostPort is the port an upstream listens on, as docker would have published
// it.
func hostPort(t *testing.T, server *httptest.Server) port.Port {
	t.Helper()

	_, portNumber, err := net.SplitHostPort(server.Listener.Addr().String())
	require.NoError(t, err)

	p, err := strconv.ParseUint(portNumber, 10, 16)
	require.NoError(t, err)

	return port.Port(p)
}

func get(t *testing.T, url string) (int, string) {
	t.Helper()

	response, err := http.Get(url)
	require.NoError(t, err)
	defer response.Body.Close()

	body, err := io.ReadAll(response.Body)
	require.NoError(t, err)

	return response.StatusCode, string(body)
}

func TestProxyHandler(t *testing.T) {
	t.Run("the request reaches the run through the runtime, as the client asked for it", func(t *testing.T) {
		var gotPath, gotQuery, gotHost string

		app := upstream(t, func(rw http.ResponseWriter, r *http.Request) {
			gotPath, gotQuery, gotHost = r.URL.Path, r.URL.RawQuery, r.Host
			_, _ = io.WriteString(rw, "served")
		})

		taskManager := &dialingRuntime{upstream: app.Listener.Addr().String()}
		holding(&taskManager.MockRuntime, runOn(8080, 80))

		server := serve(t, taskManager, "docker")

		request, err := http.NewRequest(http.MethodGet, server.URL+"/tasks/nginx-xkfqz/0/assets/app.js?v=2", nil)
		require.NoError(t, err)
		request.Host = "nginx-xkfqz.workload.localhost"

		response, err := http.DefaultClient.Do(request)
		require.NoError(t, err)
		defer response.Body.Close()

		body, _ := io.ReadAll(response.Body)

		assert.Equal(t, http.StatusOK, response.StatusCode)
		assert.Equal(t, "served", string(body))
		assert.Equal(t, "/assets/app.js", gotPath)
		assert.Equal(t, "v=2", gotQuery)
		assert.Equal(t, "nginx-xkfqz.workload.localhost", gotHost, "the task is addressed by the name the client used")
		assert.Equal(t, []string{"firecracker:0123456789abcdef/80"}, taskManager.asked(), "a bare hostname reaches the lowest port")
	})

	t.Run("a named port is the one dialled", func(t *testing.T) {
		app := upstream(t, func(rw http.ResponseWriter, r *http.Request) {})

		taskManager := &dialingRuntime{upstream: app.Listener.Addr().String()}
		holding(&taskManager.MockRuntime, runOn(80, 8080))

		status, _ := get(t, serve(t, taskManager, "docker").URL+"/tasks/nginx-xkfqz/8080/")

		assert.Equal(t, http.StatusOK, status)
		assert.Equal(t, []string{"firecracker:0123456789abcdef/8080"}, taskManager.asked())
	})

	t.Run("a connection is kept for the next request to the same port, and to that port alone", func(t *testing.T) {
		app := upstream(t, func(rw http.ResponseWriter, r *http.Request) {
			_, _ = io.WriteString(rw, "served")
		})

		taskManager := &dialingRuntime{upstream: app.Listener.Addr().String()}
		holding(&taskManager.MockRuntime, runOn(80, 8080))

		server := serve(t, taskManager, "docker")

		for range 3 {
			status, body := get(t, server.URL+"/tasks/nginx-xkfqz/80/")
			require.Equal(t, http.StatusOK, status)
			require.Equal(t, "served", body)
		}

		status, _ := get(t, server.URL+"/tasks/nginx-xkfqz/8080/")
		require.Equal(t, http.StatusOK, status)

		assert.Equal(t, []string{"firecracker:0123456789abcdef/80", "firecracker:0123456789abcdef/8080"}, taskManager.asked())
	})

	t.Run("a run the runtime cannot dial is reached where docker published it", func(t *testing.T) {
		var reached bool

		app := upstream(t, func(rw http.ResponseWriter, r *http.Request) {
			reached = true
		})

		run := publishedOn(map[port.Port]port.Port{80: hostPort(t, app)})
		run.Endpoints = []port.Port{80}

		taskManager := &dialingRuntime{err: task.ErrNotSupported}
		holding(&taskManager.MockRuntime, run)

		status, _ := get(t, serve(t, taskManager, "127.0.0.1").URL+"/tasks/nginx-xkfqz/0/")

		assert.Equal(t, http.StatusOK, status)
		assert.True(t, reached)
		assert.Equal(t, []string{"3f2a9c/80"}, taskManager.asked(), "the runtime is asked first")
	})

	t.Run("a runtime that is no dialer is reached where docker published the port, as it always was", func(t *testing.T) {
		var reached bool

		app := upstream(t, func(rw http.ResponseWriter, r *http.Request) {
			reached = true
		})

		var taskManager runtime.MockRuntime
		holding(&taskManager, publishedOn(map[port.Port]port.Port{80: hostPort(t, app)}))

		status, _ := get(t, serve(t, &taskManager, "127.0.0.1").URL+"/tasks/nginx-xkfqz/0/")

		assert.Equal(t, http.StatusOK, status)
		assert.True(t, reached)
	})

	t.Run("a websocket upgrade travels through a dialled connection", func(t *testing.T) {
		upgrader := websocket.Upgrader{}

		app := upstream(t, func(rw http.ResponseWriter, r *http.Request) {
			conn, err := upgrader.Upgrade(rw, r, nil)
			if err != nil {
				return
			}
			defer conn.Close()

			for {
				kind, message, err := conn.ReadMessage()
				if err != nil {
					return
				}

				if err := conn.WriteMessage(kind, append([]byte("echo: "), message...)); err != nil {
					return
				}
			}
		})

		taskManager := &dialingRuntime{upstream: app.Listener.Addr().String()}
		holding(&taskManager.MockRuntime, runOn(3000))

		server := serve(t, taskManager, "docker")

		conn, _, err := websocket.DefaultDialer.Dial("ws"+strings.TrimPrefix(server.URL, "http")+"/tasks/nginx-xkfqz/0/hmr", nil)
		require.NoError(t, err)
		defer conn.Close()

		for _, said := range []string{"hello", "again"} {
			require.NoError(t, conn.WriteMessage(websocket.TextMessage, []byte(said)))

			_, answered, err := conn.ReadMessage()
			require.NoError(t, err)
			assert.Equal(t, "echo: "+said, string(answered))
		}

		assert.Equal(t, []string{"firecracker:0123456789abcdef/3000"}, taskManager.asked())
	})

	t.Run("a task this node is not holding is not found", func(t *testing.T) {
		taskManager := &dialingRuntime{}
		holding(&taskManager.MockRuntime, runOn(80))

		status, _ := get(t, serve(t, taskManager, "docker").URL+"/tasks/somebody-else/0/")

		assert.Equal(t, http.StatusNotFound, status)
		assert.Empty(t, taskManager.asked())
	})

	t.Run("a port the task cannot be reached on is not found", func(t *testing.T) {
		taskManager := &dialingRuntime{}
		holding(&taskManager.MockRuntime, runOn(80))

		status, _ := get(t, serve(t, taskManager, "docker").URL+"/tasks/nginx-xkfqz/8080/")

		assert.Equal(t, http.StatusNotFound, status)
		assert.Empty(t, taskManager.asked())
	})

	t.Run("a task that is not running is unavailable", func(t *testing.T) {
		run := runOn(80)
		run.Status = task.StatusExited

		taskManager := &dialingRuntime{}
		holding(&taskManager.MockRuntime, run)

		status, _ := get(t, serve(t, taskManager, "docker").URL+"/tasks/nginx-xkfqz/0/")

		assert.Equal(t, http.StatusServiceUnavailable, status)
		assert.Empty(t, taskManager.asked())
	})

	t.Run("a run that does not answer is a bad gateway", func(t *testing.T) {
		taskManager := &dialingRuntime{err: errors.New("the run is not answering")}
		holding(&taskManager.MockRuntime, runOn(80))

		status, _ := get(t, serve(t, taskManager, "docker").URL+"/tasks/nginx-xkfqz/0/")

		assert.Equal(t, http.StatusBadGateway, status)
	})

	t.Run("something that is not a port is a bad request", func(t *testing.T) {
		taskManager := &dialingRuntime{}

		status, _ := get(t, serve(t, taskManager, "docker").URL+"/tasks/nginx-xkfqz/http/")

		assert.Equal(t, http.StatusBadRequest, status)
	})
}

func TestAddress(t *testing.T) {
	t.Run("where a request goes is read back as it was written", func(t *testing.T) {
		for _, endpoint := range []*getendpoint.Response{
			{ExecutionID: "firecracker:0123456789abcdef", Port: 8080},
			{ExecutionID: "3f2a9c", Port: 80, Host: "docker", HostPort: 32768},
			{ExecutionID: "3f2a9c", Port: 80, HostPort: 32768},
		} {
			address, err := addressOf(endpoint)
			require.NoError(t, err)

			// a host and a port, as the transport takes an address apart.
			_, portNumber, err := net.SplitHostPort(address)
			require.NoError(t, err)
			assert.Equal(t, strconv.FormatUint(uint64(endpoint.Port), 10), portNumber)

			read, err := endpointOf(address)
			require.NoError(t, err)
			assert.Equal(t, endpoint, read)
		}
	})

	t.Run("two ports, or two places a port was published, are two addresses", func(t *testing.T) {
		first, err := addressOf(&getendpoint.Response{ExecutionID: "3f2a9c", Port: 80, Host: "docker", HostPort: 32768})
		require.NoError(t, err)

		second, err := addressOf(&getendpoint.Response{ExecutionID: "3f2a9c", Port: 80, Host: "docker", HostPort: 32769})
		require.NoError(t, err)

		third, err := addressOf(&getendpoint.Response{ExecutionID: "3f2a9c", Port: 81, Host: "docker", HostPort: 32768})
		require.NoError(t, err)

		assert.NotEqual(t, first, second)
		assert.NotEqual(t, first, third)
	})

	t.Run("an address nothing wrote names nothing", func(t *testing.T) {
		_, err := endpointOf("example.com:80")

		assert.Error(t, err)
	})
}

// runOn is a run reachable through its runtime on the given ports, as a
// microVM is: published nowhere.
func runOn(endpoints ...port.Port) task.Execution {
	return task.Execution{
		ID:        "firecracker:0123456789abcdef",
		Status:    task.StatusRunning,
		Kind:      task.KindService,
		Endpoints: endpoints,
	}
}

// publishedOn is a run docker published, as docker runs one.
func publishedOn(bindings map[port.Port]port.Port) task.Execution {
	portBindings := make(port.PortMap, len(bindings))
	for taskPort, published := range bindings {
		portBindings[taskPort] = []port.PortBinding{{HostIP: "0.0.0.0", HostPort: published}}
	}

	return task.Execution{
		ID:           "3f2a9c",
		Status:       task.StatusRunning,
		Kind:         task.KindService,
		PortBindings: portBindings,
	}
}
