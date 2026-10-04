package getEndpoint

import (
	"errors"
	"io"
	"net"
	"strconv"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/khanzadimahdi/testproject/domain/workload/port"
	"github.com/khanzadimahdi/testproject/domain/workload/task"
	"github.com/khanzadimahdi/testproject/infrastructure/repository/mocks/workload/runtime"
)

// published builds a run this node is holding as docker runs one: reachable
// where its ports were published, which is all docker says about them.
func published(bindings map[port.Port]port.Port) task.Execution {
	portBindings := make(port.PortMap, len(bindings))
	for taskPort, hostPort := range bindings {
		portBindings[taskPort] = []port.PortBinding{{HostIP: "0.0.0.0", HostPort: hostPort}}
	}

	return task.Execution{
		ID:           "task-1",
		Status:       task.StatusRunning,
		PortBindings: portBindings,
		Kind:         task.KindService,
	}
}

// reachableOn builds a run this node is holding as a runtime that says which of
// its ports are reachable does, a microVM's say: published nowhere.
func reachableOn(endpoints ...port.Port) task.Execution {
	return task.Execution{
		ID:        "firecracker:0123456789abcdef",
		Status:    task.StatusRunning,
		Endpoints: endpoints,
		Kind:      task.KindService,
	}
}

func TestUseCase_Execute(t *testing.T) {
	t.Run("a bare request reaches the lowest port docker published", func(t *testing.T) {
		var taskManager runtime.MockRuntime
		taskManager.On("BySlug", mock.Anything, "nginx-xkfqz").Once().
			Return([]task.Execution{published(map[port.Port]port.Port{8080: 32769, 80: 32768})}, nil)
		defer taskManager.AssertExpectations(t)

		response, err := NewUseCase(&taskManager, "docker").Execute(t.Context(), &Request{Slug: "nginx-xkfqz"})

		assert.NoError(t, err)
		assert.Equal(t, &Response{ExecutionID: "task-1", Port: 80, Host: "docker", HostPort: 32768}, response)
		assert.Equal(t, "docker:32768", response.Address())
	})

	t.Run("a named port reaches that port", func(t *testing.T) {
		var taskManager runtime.MockRuntime
		taskManager.On("BySlug", mock.Anything, "nginx-xkfqz").Once().
			Return([]task.Execution{published(map[port.Port]port.Port{80: 32768, 8080: 32769})}, nil)
		defer taskManager.AssertExpectations(t)

		response, err := NewUseCase(&taskManager, "docker").Execute(t.Context(), &Request{Slug: "nginx-xkfqz", Port: 8080})

		assert.NoError(t, err)
		assert.Equal(t, &Response{ExecutionID: "task-1", Port: 8080, Host: "docker", HostPort: 32769}, response)
	})

	t.Run("a port the task does not expose is not there", func(t *testing.T) {
		var taskManager runtime.MockRuntime
		taskManager.On("BySlug", mock.Anything, "nginx-xkfqz").Once().
			Return([]task.Execution{published(map[port.Port]port.Port{80: 32768})}, nil)
		defer taskManager.AssertExpectations(t)

		_, err := NewUseCase(&taskManager, "docker").Execute(t.Context(), &Request{Slug: "nginx-xkfqz", Port: 9999})

		assert.ErrorIs(t, err, ErrNotExposed)
	})

	t.Run("a task that publishes nothing, on a runtime that cannot dial it, cannot be reached", func(t *testing.T) {
		c := published(nil)
		c.PortBindings = port.PortMap{80: []port.PortBinding{{HostPort: 0}}}

		var taskManager runtime.MockRuntime
		taskManager.On("BySlug", mock.Anything, "nginx-xkfqz").Once().
			Return([]task.Execution{c}, nil)
		defer taskManager.AssertExpectations(t)

		_, err := NewUseCase(&taskManager, "docker").Execute(t.Context(), &Request{Slug: "nginx-xkfqz"})

		assert.ErrorIs(t, err, ErrNotExposed)
	})

	t.Run("a runtime that says which ports are reachable is taken at its word", func(t *testing.T) {
		var taskManager runtime.MockDialingRuntime
		taskManager.On("BySlug", mock.Anything, "nginx-xkfqz").Once().
			Return([]task.Execution{reachableOn(8080, 80)}, nil)
		defer taskManager.AssertExpectations(t)

		response, err := NewUseCase(&taskManager, "docker").Execute(t.Context(), &Request{Slug: "nginx-xkfqz"})

		assert.NoError(t, err)
		assert.Equal(t, &Response{ExecutionID: "firecracker:0123456789abcdef", Port: 80}, response)
		assert.False(t, response.Published(), "a port reached through the runtime is published nowhere")
	})

	t.Run("a port the runtime cannot reach right now is not there", func(t *testing.T) {
		var taskManager runtime.MockDialingRuntime
		taskManager.On("BySlug", mock.Anything, "nginx-xkfqz").Once().
			Return([]task.Execution{reachableOn(80)}, nil)
		defer taskManager.AssertExpectations(t)

		_, err := NewUseCase(&taskManager, "docker").Execute(t.Context(), &Request{Slug: "nginx-xkfqz", Port: 8080})

		assert.ErrorIs(t, err, ErrNotExposed)
	})

	t.Run("a reachable port that was published says where too", func(t *testing.T) {
		c := published(map[port.Port]port.Port{80: 32768})
		c.Endpoints = []port.Port{80}

		var taskManager runtime.MockDialingRuntime
		taskManager.On("BySlug", mock.Anything, "nginx-xkfqz").Once().
			Return([]task.Execution{c}, nil)
		defer taskManager.AssertExpectations(t)

		response, err := NewUseCase(&taskManager, "docker").Execute(t.Context(), &Request{Slug: "nginx-xkfqz"})

		assert.NoError(t, err)
		assert.Equal(t, &Response{ExecutionID: "task-1", Port: 80, Host: "docker", HostPort: 32768}, response)
	})

	t.Run("a runtime that cannot dial reaches only what it published, whatever it says is reachable", func(t *testing.T) {
		c := published(map[port.Port]port.Port{8080: 32769})
		c.Endpoints = []port.Port{80, 8080}

		var taskManager runtime.MockRuntime
		taskManager.On("BySlug", mock.Anything, "nginx-xkfqz").Twice().
			Return([]task.Execution{c}, nil)
		defer taskManager.AssertExpectations(t)

		useCase := NewUseCase(&taskManager, "docker")

		response, err := useCase.Execute(t.Context(), &Request{Slug: "nginx-xkfqz"})
		assert.NoError(t, err)
		assert.Equal(t, &Response{ExecutionID: "task-1", Port: 8080, Host: "docker", HostPort: 32769}, response)

		_, err = useCase.Execute(t.Context(), &Request{Slug: "nginx-xkfqz", Port: 80})
		assert.ErrorIs(t, err, ErrNotExposed)
	})

	t.Run("the latest run that is running is the one reached", func(t *testing.T) {
		now := time.Now()

		leaving := reachableOn(80)
		leaving.ID, leaving.Status, leaving.CreatedAt = "firecracker:000000000000000a", task.StatusExited, now

		older := reachableOn(80)
		older.ID, older.CreatedAt = "firecracker:000000000000000b", now.Add(-time.Hour)

		newer := reachableOn(80)
		newer.ID, newer.CreatedAt = "firecracker:000000000000000c", now.Add(-time.Minute)

		var taskManager runtime.MockDialingRuntime
		taskManager.On("BySlug", mock.Anything, "nginx-xkfqz").Once().
			Return([]task.Execution{newer, older, leaving}, nil)
		defer taskManager.AssertExpectations(t)

		response, err := NewUseCase(&taskManager, "docker").Execute(t.Context(), &Request{Slug: "nginx-xkfqz"})

		assert.NoError(t, err)
		assert.Equal(t, "firecracker:000000000000000c", response.ExecutionID)
	})

	t.Run("a task this node is not holding says so", func(t *testing.T) {
		var taskManager runtime.MockRuntime
		taskManager.On("BySlug", mock.Anything, "nginx-xkfqz").Once().
			Return([]task.Execution{}, nil)
		defer taskManager.AssertExpectations(t)

		_, err := NewUseCase(&taskManager, "docker").Execute(t.Context(), &Request{Slug: "nginx-xkfqz"})

		assert.ErrorIs(t, err, ErrNotHeld)
	})

	t.Run("a task that has stopped says so", func(t *testing.T) {
		c := published(map[port.Port]port.Port{80: 32768})
		c.Status = task.StatusExited

		var taskManager runtime.MockRuntime
		taskManager.On("BySlug", mock.Anything, "nginx-xkfqz").Once().
			Return([]task.Execution{c}, nil)
		defer taskManager.AssertExpectations(t)

		_, err := NewUseCase(&taskManager, "docker").Execute(t.Context(), &Request{Slug: "nginx-xkfqz"})

		assert.ErrorIs(t, err, ErrNotRunning)
	})

	t.Run("the runtime failing fails the use case", func(t *testing.T) {
		expected := errors.New("the runtime is not answering")

		var taskManager runtime.MockRuntime
		taskManager.On("BySlug", mock.Anything, "nginx-xkfqz").Once().
			Return([]task.Execution(nil), expected)
		defer taskManager.AssertExpectations(t)

		_, err := NewUseCase(&taskManager, "docker").Execute(t.Context(), &Request{Slug: "nginx-xkfqz"})

		assert.ErrorIs(t, err, expected)
	})
}

// listen stands for a port a run was published on: it accepts one connection
// and says hello on it.
func listen(t *testing.T) port.Port {
	t.Helper()

	listener, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	t.Cleanup(func() { _ = listener.Close() })

	go func() {
		conn, err := listener.Accept()
		if err != nil {
			return
		}
		defer conn.Close()

		_, _ = io.WriteString(conn, "hello")
	}()

	_, portNumber, err := net.SplitHostPort(listener.Addr().String())
	require.NoError(t, err)

	p, err := strconv.ParseUint(portNumber, 10, 16)
	require.NoError(t, err)

	return port.Port(p)
}

func greeting(t *testing.T, conn net.Conn) string {
	t.Helper()
	defer conn.Close()

	said, err := io.ReadAll(conn)
	require.NoError(t, err)

	return string(said)
}

func TestUseCase_Dial(t *testing.T) {
	t.Run("a runtime that reaches its runs itself is asked for the run's own port", func(t *testing.T) {
		near, far := net.Pipe()
		defer far.Close()

		var taskManager runtime.MockDialingRuntime
		taskManager.On("DialContext", mock.Anything, "firecracker:0123456789abcdef", port.Port(80)).Once().Return(near, nil)
		defer taskManager.AssertExpectations(t)

		conn, err := NewUseCase(&taskManager, "docker").Dial(t.Context(), &Response{ExecutionID: "firecracker:0123456789abcdef", Port: 80})

		require.NoError(t, err)
		assert.Same(t, near, conn)
	})

	t.Run("a run the runtime cannot dial is reached where its port was published", func(t *testing.T) {
		hostPort := listen(t)

		var taskManager runtime.MockDialingRuntime
		taskManager.On("DialContext", mock.Anything, "task-1", port.Port(80)).Once().Return(nil, task.ErrNotSupported)
		defer taskManager.AssertExpectations(t)

		conn, err := NewUseCase(&taskManager, "127.0.0.1").Dial(t.Context(), &Response{ExecutionID: "task-1", Port: 80, Host: "127.0.0.1", HostPort: hostPort})

		require.NoError(t, err)
		assert.Equal(t, "hello", greeting(t, conn))
	})

	t.Run("a runtime that is no dialer is reached where the port was published", func(t *testing.T) {
		hostPort := listen(t)

		var taskManager runtime.MockRuntime
		defer taskManager.AssertExpectations(t)

		conn, err := NewUseCase(&taskManager, "127.0.0.1").Dial(t.Context(), &Response{ExecutionID: "task-1", Port: 80, Host: "127.0.0.1", HostPort: hostPort})

		require.NoError(t, err)
		assert.Equal(t, "hello", greeting(t, conn))
	})

	t.Run("a run the runtime cannot dial, published nowhere, cannot be reached", func(t *testing.T) {
		var taskManager runtime.MockDialingRuntime
		taskManager.On("DialContext", mock.Anything, "task-1", port.Port(80)).Once().Return(nil, task.ErrNotSupported)
		defer taskManager.AssertExpectations(t)

		_, err := NewUseCase(&taskManager, "docker").Dial(t.Context(), &Response{ExecutionID: "task-1", Port: 80})

		assert.ErrorIs(t, err, ErrNotExposed)
	})

	t.Run("a runtime that fails to dial a run is not gone around", func(t *testing.T) {
		expected := errors.New("the run is not answering")

		var taskManager runtime.MockDialingRuntime
		taskManager.On("DialContext", mock.Anything, "task-1", port.Port(80)).Once().Return(nil, expected)
		defer taskManager.AssertExpectations(t)

		_, err := NewUseCase(&taskManager, "docker").Dial(t.Context(), &Response{ExecutionID: "task-1", Port: 80, Host: "docker", HostPort: 32768})

		assert.ErrorIs(t, err, expected)
	})
}

func TestRequest_Validate(t *testing.T) {
	t.Run("a request naming a task is valid", func(t *testing.T) {
		assert.Empty(t, (&Request{Slug: "nginx-xkfqz"}).Validate())
	})

	t.Run("a request naming none is not", func(t *testing.T) {
		assert.Equal(t, "required_field", (&Request{}).Validate()["slug"])
	})
}
