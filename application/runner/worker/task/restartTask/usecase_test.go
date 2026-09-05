package restartTask

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/khanzadimahdi/testproject/domain"
	"github.com/khanzadimahdi/testproject/domain/runner/container"
	"github.com/khanzadimahdi/testproject/infrastructure/repository/mocks/runner/containers"
	"github.com/khanzadimahdi/testproject/infrastructure/validator"
)

func accepts() *validator.MockValidator {
	v := &validator.MockValidator{}
	v.On("Validate", mock.Anything).Return(domain.ValidationErrors{})

	return v
}

func TestUseCase_Execute(t *testing.T) {
	t.Parallel()

	t.Run("restarts the containers a task is running", func(t *testing.T) {
		t.Parallel()

		var containerManager containers.MockContainerManager

		containerManager.On("GetByLabel", mock.Anything, container.TaskUUIDLabelKey, "task-uuid").
			Return([]container.Container{{ID: "container-1"}, {ID: "container-2"}}, nil).Once()
		containerManager.On("Restart", mock.Anything, "container-1").Return(nil).Once()
		containerManager.On("Restart", mock.Anything, "container-2").Return(nil).Once()
		defer containerManager.AssertExpectations(t)

		_, err := NewUseCase(&containerManager, accepts()).
			Execute(context.Background(), &Request{UUID: "task-uuid"})

		assert.NoError(t, err)
	})

	t.Run("a task with no container on this node is not this node's to restart", func(t *testing.T) {
		t.Parallel()

		var containerManager containers.MockContainerManager

		containerManager.On("GetByLabel", mock.Anything, container.TaskUUIDLabelKey, "task-uuid").
			Return([]container.Container{}, nil).Once()
		defer containerManager.AssertExpectations(t)

		_, err := NewUseCase(&containerManager, accepts()).
			Execute(context.Background(), &Request{UUID: "task-uuid"})

		assert.ErrorIs(t, err, domain.ErrNotExists)
		containerManager.AssertNotCalled(t, "Restart", mock.Anything, mock.Anything)
	})

	t.Run("a container that will not come back is reported", func(t *testing.T) {
		t.Parallel()

		var containerManager containers.MockContainerManager

		expected := errors.New("the daemon is unreachable")

		containerManager.On("GetByLabel", mock.Anything, container.TaskUUIDLabelKey, "task-uuid").
			Return([]container.Container{{ID: "container-1"}}, nil).Once()
		containerManager.On("Restart", mock.Anything, "container-1").Return(expected).Once()

		_, err := NewUseCase(&containerManager, accepts()).
			Execute(context.Background(), &Request{UUID: "task-uuid"})

		assert.ErrorIs(t, err, expected)
	})

	t.Run("a request the rules refuse never reaches the daemon", func(t *testing.T) {
		t.Parallel()

		var containerManager containers.MockContainerManager

		refusal := domain.ValidationErrors{"uuid": "required_field"}

		v := &validator.MockValidator{}
		v.On("Validate", mock.Anything).Return(refusal)

		response, err := NewUseCase(&containerManager, v).Execute(context.Background(), &Request{})

		require.NoError(t, err)
		assert.Equal(t, refusal, response.ValidationErrors)

		containerManager.AssertNotCalled(t, "GetByLabel", mock.Anything, mock.Anything, mock.Anything)
	})
}
