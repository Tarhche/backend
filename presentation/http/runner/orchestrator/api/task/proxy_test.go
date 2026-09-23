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
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	getendpoint "github.com/khanzadimahdi/testproject/application/runner/orchestrator/task/getEndpoint"
	"github.com/khanzadimahdi/testproject/domain/runner/port"
	"github.com/khanzadimahdi/testproject/domain/runner/task"
	"github.com/khanzadimahdi/testproject/infrastructure/repository/mocks/runner/runtime"
)

// dialer stands for the runtime: asked for a run and one of its ports, it
// answers with a connection to the server standing in for that run.
type dialer struct {
	upstream string
	dialled  []string
}

func (d *dialer) Dial(ctx context.Context, executionID string, p port.Port) (net.Conn, error) {
	d.dialled = append(d.dialled, net.JoinHostPort(executionID, strconv.FormatUint(uint64(p), 10)))

	if len(d.upstream) == 0 {
		return nil, errors.New("the run is not answering")
	}

	return (&net.Dialer{}).DialContext(ctx, "tcp", d.upstream)
}

// serve builds the route the way the provider does, over a runtime holding one
// running task reachable on the given ports.
func serve(t *testing.T, d *dialer, endpoints ...port.Port) *httptest.Server {
	t.Helper()

	var taskManager runtime.MockRuntime
	taskManager.On("BySlug", mock.Anything, "nginx-xkfqz").Return([]task.Execution{{
		ID:        "0123456789abcdef",
		Status:    task.StatusRunning,
		Kind:      task.KindService,
		Endpoints: endpoints,
	}}, nil)
	taskManager.On("BySlug", mock.Anything, mock.Anything).Return([]task.Execution{}, nil)

	mux := http.NewServeMux()
	mux.Handle("/tasks/{slug}/{port}/{path...}", NewProxyHandler(getendpoint.NewUseCase(&taskManager), d, slog.New(slog.DiscardHandler)))

	server := httptest.NewServer(mux)
	t.Cleanup(server.Close)

	return server
}

func TestProxyHandler(t *testing.T) {
	t.Run("the request reaches the run through the runtime, as the client asked for it", func(t *testing.T) {
		var gotPath, gotQuery, gotHost string

		upstream := httptest.NewServer(http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
			gotPath, gotQuery, gotHost = r.URL.Path, r.URL.RawQuery, r.Host
			_, _ = io.WriteString(rw, "served")
		}))
		t.Cleanup(upstream.Close)

		d := &dialer{upstream: upstream.Listener.Addr().String()}
		server := serve(t, d, 8080, 80)

		request, err := http.NewRequest(http.MethodGet, server.URL+"/tasks/nginx-xkfqz/0/assets/app.js?v=2", nil)
		require.NoError(t, err)
		request.Host = "nginx-xkfqz.runner.localhost"

		response, err := http.DefaultClient.Do(request)
		require.NoError(t, err)
		defer response.Body.Close()

		body, _ := io.ReadAll(response.Body)

		assert.Equal(t, http.StatusOK, response.StatusCode)
		assert.Equal(t, "served", string(body))
		assert.Equal(t, "/assets/app.js", gotPath)
		assert.Equal(t, "v=2", gotQuery)
		assert.Equal(t, "nginx-xkfqz.runner.localhost", gotHost, "the task is addressed by the name the client used")
		assert.Equal(t, []string{"0123456789abcdef:80"}, d.dialled, "a bare hostname reaches the lowest port")
	})

	t.Run("a named port is the one dialled", func(t *testing.T) {
		upstream := httptest.NewServer(http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {}))
		t.Cleanup(upstream.Close)

		d := &dialer{upstream: upstream.Listener.Addr().String()}
		server := serve(t, d, 80, 8080)

		response, err := http.Get(server.URL + "/tasks/nginx-xkfqz/8080/")
		require.NoError(t, err)
		defer response.Body.Close()

		assert.Equal(t, http.StatusOK, response.StatusCode)
		assert.Equal(t, []string{"0123456789abcdef:8080"}, d.dialled)
	})

	t.Run("a task this node is not holding is not found", func(t *testing.T) {
		d := &dialer{}
		server := serve(t, d, 80)

		response, err := http.Get(server.URL + "/tasks/somebody-else/0/")
		require.NoError(t, err)
		defer response.Body.Close()

		assert.Equal(t, http.StatusNotFound, response.StatusCode)
		assert.Empty(t, d.dialled)
	})

	t.Run("a run that does not answer is a bad gateway", func(t *testing.T) {
		d := &dialer{}
		server := serve(t, d, 80)

		response, err := http.Get(server.URL + "/tasks/nginx-xkfqz/0/")
		require.NoError(t, err)
		defer response.Body.Close()

		assert.Equal(t, http.StatusBadGateway, response.StatusCode)
	})
}
