package restartTask

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/khanzadimahdi/testproject/domain"
	"github.com/khanzadimahdi/testproject/domain/runner/task"
	"github.com/khanzadimahdi/testproject/infrastructure/repository/mocks/runner/runtime"
	"github.com/khanzadimahdi/testproject/infrastructure/validator"
)

func accepts() *validator.MockValidator {
	v := &validator.MockValidator{}
	v.On("Validate", mock.Anything).Return(domain.ValidationErrors{})

	return v
}

func TestUseCase_Execute(t *testing.T) {
	t.Parallel()

	t.Run("restarts the tasks a task is running", func(t *testing.T) {
		t.Parallel()

		var taskManager runtime.MockRuntime

		taskManager.On("Of", mock.Anything, "task-uuid").
			Return([]task.Execution{{ID: "task-1"}, {ID: "task-2"}}, nil).Once()
		taskManager.On("Restart", mock.Anything, "task-1").Return(nil).Once()
		taskManager.On("Restart", mock.Anything, "task-2").Return(nil).Once()
		defer taskManager.AssertExpectations(t)

		_, err := NewUseCase(&taskManager, accepts()).
			Execute(context.Background(), &Request{UUID: "task-uuid"})

		assert.NoError(t, err)
	})

	t.Run("a task with no task on this node is not this node's to restart", func(t *testing.T) {
		t.Parallel()

		var taskManager runtime.MockRuntime

		taskManager.On("Of", mock.Anything, "task-uuid").
			Return([]task.Execution{}, nil).Once()
		defer taskManager.AssertExpectations(t)

		_, err := NewUseCase(&taskManager, accepts()).
			Execute(context.Background(), &Request{UUID: "task-uuid"})

		assert.ErrorIs(t, err, domain.ErrNotExists)
		taskManager.AssertNotCalled(t, "Restart", mock.Anything, mock.Anything)
	})

	t.Run("a task that will not come back is reported", func(t *testing.T) {
		t.Parallel()

		var taskManager runtime.MockRuntime

		expected := errors.New("the daemon is unreachable")

		taskManager.On("Of", mock.Anything, "task-uuid").
			Return([]task.Execution{{ID: "task-1"}}, nil).Once()
		taskManager.On("Restart", mock.Anything, "task-1").Return(expected).Once()

		_, err := NewUseCase(&taskManager, accepts()).
			Execute(context.Background(), &Request{UUID: "task-uuid"})

		assert.ErrorIs(t, err, expected)
	})

	t.Run("a request the rules refuse never reaches the daemon", func(t *testing.T) {
		t.Parallel()

		var taskManager runtime.MockRuntime

		refusal := domain.ValidationErrors{"uuid": "required_field"}

		v := &validator.MockValidator{}
		v.On("Validate", mock.Anything).Return(refusal)

		response, err := NewUseCase(&taskManager, v).Execute(context.Background(), &Request{})

		require.NoError(t, err)
		assert.Equal(t, refusal, response.ValidationErrors)

		taskManager.AssertNotCalled(t, "Of", mock.Anything, mock.Anything)
	})
}
