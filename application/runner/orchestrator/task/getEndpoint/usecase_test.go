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

// running builds a task this node is holding, reachable on the given ports.
func running(endpoints ...port.Port) task.Execution {
	return task.Execution{
		ID:        "task-1",
		Status:    task.StatusRunning,
		Endpoints: endpoints,
		Kind:      task.KindService,
	}
}

func TestUseCase_Execute(t *testing.T) {
	t.Run("a bare request reaches the lowest reachable port", func(t *testing.T) {
		var taskManager runtime.MockRuntime
		taskManager.On("BySlug", mock.Anything, "nginx-xkfqz").Once().
			Return([]task.Execution{running(8080, 80)}, nil)
		defer taskManager.AssertExpectations(t)

		response, err := NewUseCase(&taskManager).Execute(t.Context(), &Request{Slug: "nginx-xkfqz"})

		assert.NoError(t, err)
		assert.Equal(t, &Response{ExecutionID: "task-1", Port: 80}, response)
	})

	t.Run("a named port reaches that port", func(t *testing.T) {
		var taskManager runtime.MockRuntime
		taskManager.On("BySlug", mock.Anything, "nginx-xkfqz").Once().
			Return([]task.Execution{running(80, 8080)}, nil)
		defer taskManager.AssertExpectations(t)

		response, err := NewUseCase(&taskManager).Execute(t.Context(), &Request{Slug: "nginx-xkfqz", Port: 8080})

		assert.NoError(t, err)
		assert.Equal(t, &Response{ExecutionID: "task-1", Port: 8080}, response)
	})

	t.Run("a port the task does not expose is not there", func(t *testing.T) {
		var taskManager runtime.MockRuntime
		taskManager.On("BySlug", mock.Anything, "nginx-xkfqz").Once().
			Return([]task.Execution{running(80)}, nil)
		defer taskManager.AssertExpectations(t)

		_, err := NewUseCase(&taskManager).Execute(t.Context(), &Request{Slug: "nginx-xkfqz", Port: 9999})

		assert.ErrorIs(t, err, ErrNotExposed)
	})

	t.Run("a task reachable on nothing cannot be reached", func(t *testing.T) {
		c := running()

		var taskManager runtime.MockRuntime
		taskManager.On("BySlug", mock.Anything, "nginx-xkfqz").Once().
			Return([]task.Execution{c}, nil)
		defer taskManager.AssertExpectations(t)

		_, err := NewUseCase(&taskManager).Execute(t.Context(), &Request{Slug: "nginx-xkfqz"})

		assert.ErrorIs(t, err, ErrNotExposed)
	})

	t.Run("a task this node is not holding says so", func(t *testing.T) {
		var taskManager runtime.MockRuntime
		taskManager.On("BySlug", mock.Anything, "nginx-xkfqz").Once().
			Return([]task.Execution{}, nil)
		defer taskManager.AssertExpectations(t)

		_, err := NewUseCase(&taskManager).Execute(t.Context(), &Request{Slug: "nginx-xkfqz"})

		assert.ErrorIs(t, err, ErrNotHeld)
	})

	t.Run("a task that has stopped says so", func(t *testing.T) {
		c := running(80)
		c.Status = task.StatusExited

		var taskManager runtime.MockRuntime
		taskManager.On("BySlug", mock.Anything, "nginx-xkfqz").Once().
			Return([]task.Execution{c}, nil)
		defer taskManager.AssertExpectations(t)

		_, err := NewUseCase(&taskManager).Execute(t.Context(), &Request{Slug: "nginx-xkfqz"})

		assert.ErrorIs(t, err, ErrNotRunning)
	})

	t.Run("the runtime failing fails the use case", func(t *testing.T) {
		expected := errors.New("the runtime is not answering")

		var taskManager runtime.MockRuntime
		taskManager.On("BySlug", mock.Anything, "nginx-xkfqz").Once().
			Return([]task.Execution(nil), expected)
		defer taskManager.AssertExpectations(t)

		_, err := NewUseCase(&taskManager).Execute(t.Context(), &Request{Slug: "nginx-xkfqz"})

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
