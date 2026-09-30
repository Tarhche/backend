package getEndpoint

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"

	"github.com/khanzadimahdi/testproject/domain/runner/port"
	"github.com/khanzadimahdi/testproject/domain/runner/task"
	"github.com/khanzadimahdi/testproject/infrastructure/repository/mocks/runner/runtime"
)

// running builds a task this node is holding, with the ports docker
// published it on.
func running(published map[port.Port]port.Port) task.Execution {
	bindings := make(port.PortMap, len(published))
	for taskPort, hostPort := range published {
		bindings[taskPort] = []port.PortBinding{{HostIP: "0.0.0.0", HostPort: hostPort}}
	}

	return task.Execution{
		ID:           "task-1",
		Status:       task.StatusRunning,
		PortBindings: bindings,
		Kind:         task.KindService,
	}
}

func TestUseCase_Execute(t *testing.T) {
	t.Run("a bare request reaches the lowest published port", func(t *testing.T) {
		var taskManager runtime.MockRuntime
		taskManager.On("BySlug", mock.Anything, "nginx-xkfqz").Once().
			Return([]task.Execution{running(map[port.Port]port.Port{8080: 32769, 80: 32768})}, nil)
		defer taskManager.AssertExpectations(t)

		response, err := NewUseCase(&taskManager, "docker").Execute(t.Context(), &Request{Slug: "nginx-xkfqz"})

		assert.NoError(t, err)
		assert.Equal(t, &Response{Host: "docker", Port: 32768}, response)
		assert.Equal(t, "docker:32768", response.Address())
	})

	t.Run("a named port reaches that port", func(t *testing.T) {
		var taskManager runtime.MockRuntime
		taskManager.On("BySlug", mock.Anything, "nginx-xkfqz").Once().
			Return([]task.Execution{running(map[port.Port]port.Port{80: 32768, 8080: 32769})}, nil)
		defer taskManager.AssertExpectations(t)

		response, err := NewUseCase(&taskManager, "docker").Execute(t.Context(), &Request{Slug: "nginx-xkfqz", Port: 8080})

		assert.NoError(t, err)
		assert.Equal(t, &Response{Host: "docker", Port: 32769}, response)
	})

	t.Run("a port the task does not expose is not there", func(t *testing.T) {
		var taskManager runtime.MockRuntime
		taskManager.On("BySlug", mock.Anything, "nginx-xkfqz").Once().
			Return([]task.Execution{running(map[port.Port]port.Port{80: 32768})}, nil)
		defer taskManager.AssertExpectations(t)

		_, err := NewUseCase(&taskManager, "docker").Execute(t.Context(), &Request{Slug: "nginx-xkfqz", Port: 9999})

		assert.ErrorIs(t, err, ErrNotExposed)
	})

	t.Run("a task that publishes nothing cannot be reached", func(t *testing.T) {
		c := running(nil)
		c.PortBindings = port.PortMap{80: []port.PortBinding{{HostPort: 0}}}

		var taskManager runtime.MockRuntime
		taskManager.On("BySlug", mock.Anything, "nginx-xkfqz").Once().
			Return([]task.Execution{c}, nil)
		defer taskManager.AssertExpectations(t)

		_, err := NewUseCase(&taskManager, "docker").Execute(t.Context(), &Request{Slug: "nginx-xkfqz"})

		assert.ErrorIs(t, err, ErrNotExposed)
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
		c := running(map[port.Port]port.Port{80: 32768})
		c.Status = task.StatusExited

		var taskManager runtime.MockRuntime
		taskManager.On("BySlug", mock.Anything, "nginx-xkfqz").Once().
			Return([]task.Execution{c}, nil)
		defer taskManager.AssertExpectations(t)

		_, err := NewUseCase(&taskManager, "docker").Execute(t.Context(), &Request{Slug: "nginx-xkfqz"})

		assert.ErrorIs(t, err, ErrNotRunning)
	})

	t.Run("docker failing fails the use case", func(t *testing.T) {
		expected := errors.New("docker is not answering")

		var taskManager runtime.MockRuntime
		taskManager.On("BySlug", mock.Anything, "nginx-xkfqz").Once().
			Return([]task.Execution(nil), expected)
		defer taskManager.AssertExpectations(t)

		_, err := NewUseCase(&taskManager, "docker").Execute(t.Context(), &Request{Slug: "nginx-xkfqz"})

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
