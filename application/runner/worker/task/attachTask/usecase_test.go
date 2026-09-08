package attachTask

import (
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

// held is a running container belonging to somebody.
func held(owner string) []container.Container {
	return []container.Container{{
		ID:     "container-id",
		Status: container.StatusRunning,
		Labels: map[string]string{
			container.TaskUUIDLabelKey:  "task-uuid",
			container.TaskOwnerLabelKey: owner,
		},
	}}
}

func TestUseCase_Execute(t *testing.T) {
	t.Parallel()

	t.Run("a terminal is opened in the owner's own container", func(t *testing.T) {
		t.Parallel()

		var manager containers.MockContainerManager
		manager.On("GetByLabel", mock.Anything, container.TaskUUIDLabelKey, "task-uuid").
			Once().Return(held("owner-uuid"), nil)
		manager.On("Exec", mock.Anything, "container-id", mock.Anything).
			Once().Return(container.ExecSession(nil), nil)
		defer manager.AssertExpectations(t)

		session, validationErrors, err := NewUseCase(&manager, accepts()).
			Execute(t.Context(), &Request{UUID: "task-uuid", OwnerUUID: "owner-uuid"})

		require.NoError(t, err)
		assert.Empty(t, validationErrors)
		assert.Nil(t, session)
	})

	t.Run("somebody else's container is not there for them", func(t *testing.T) {
		t.Parallel()

		var manager containers.MockContainerManager
		manager.On("GetByLabel", mock.Anything, container.TaskUUIDLabelKey, "task-uuid").
			Once().Return(held("somebody-else"), nil)
		defer manager.AssertExpectations(t)

		_, _, err := NewUseCase(&manager, accepts()).
			Execute(t.Context(), &Request{UUID: "task-uuid", OwnerUUID: "owner-uuid"})

		// not "forbidden": a container that is not yours is one you cannot see,
		// so knowing a uuid tells the asker nothing about whether it exists.
		assert.ErrorIs(t, err, domain.ErrNotExists)
		manager.AssertNotCalled(t, "Exec", mock.Anything, mock.Anything, mock.Anything)
	})

	t.Run("a container with no owner is open to anybody", func(t *testing.T) {
		t.Parallel()

		// a snippet: it belongs to nobody, so it is everybody's, and nobody has
		// to say who they are to reach it
		for _, asking := range []string{"", "somebody", "somebody-else"} {
			t.Run("asked by "+asking, func(t *testing.T) {
				var manager containers.MockContainerManager
				manager.On("GetByLabel", mock.Anything, container.TaskUUIDLabelKey, "task-uuid").
					Once().Return(held(""), nil)
				manager.On("Exec", mock.Anything, "container-id", mock.Anything).
					Once().Return(container.ExecSession(nil), nil)
				defer manager.AssertExpectations(t)

				_, validationErrors, err := NewUseCase(&manager, accepts()).
					Execute(t.Context(), &Request{UUID: "task-uuid", OwnerUUID: asking})

				require.NoError(t, err)
				assert.Empty(t, validationErrors)
			})
		}
	})

	t.Run("an owned container is not open to nobody", func(t *testing.T) {
		t.Parallel()

		var manager containers.MockContainerManager
		manager.On("GetByLabel", mock.Anything, container.TaskUUIDLabelKey, "task-uuid").
			Once().Return(held("owner-uuid"), nil)
		defer manager.AssertExpectations(t)

		_, _, err := NewUseCase(&manager, accepts()).
			Execute(t.Context(), &Request{UUID: "task-uuid", OwnerUUID: ""})

		assert.ErrorIs(t, err, domain.ErrNotExists,
			"an owner is what makes a container private, and anonymous is not that owner")
		manager.AssertNotCalled(t, "Exec", mock.Anything, mock.Anything, mock.Anything)
	})

	t.Run("a task with no container is not there", func(t *testing.T) {
		t.Parallel()

		var manager containers.MockContainerManager
		manager.On("GetByLabel", mock.Anything, container.TaskUUIDLabelKey, "task-uuid").
			Once().Return([]container.Container{}, nil)
		defer manager.AssertExpectations(t)

		_, _, err := NewUseCase(&manager, accepts()).
			Execute(t.Context(), &Request{UUID: "task-uuid", OwnerUUID: "owner-uuid"})

		assert.ErrorIs(t, err, domain.ErrNotExists)
	})

	t.Run("the owner's container has to be running", func(t *testing.T) {
		t.Parallel()

		stopped := held("owner-uuid")
		stopped[0].Status = container.StatusExited

		var manager containers.MockContainerManager
		manager.On("GetByLabel", mock.Anything, container.TaskUUIDLabelKey, "task-uuid").
			Once().Return(stopped, nil)
		defer manager.AssertExpectations(t)

		_, validationErrors, err := NewUseCase(&manager, accepts()).
			Execute(t.Context(), &Request{UUID: "task-uuid", OwnerUUID: "owner-uuid"})

		require.NoError(t, err)
		assert.Equal(t, domain.ValidationErrors{"uuid": "container_is_not_running"}, validationErrors)
	})
}

func TestRequest_Validate(t *testing.T) {
	t.Parallel()

	t.Run("a uuid is required, and who is asking is not", func(t *testing.T) {
		t.Parallel()

		assert.Empty(t, (&Request{UUID: "task-uuid", OwnerUUID: "owner-uuid"}).Validate())

		// nobody is a caller too: a snippet has no owner to check against
		assert.Empty(t, (&Request{UUID: "task-uuid"}).Validate())

		assert.Equal(t, domain.ValidationErrors{"uuid": "required_field"},
			(&Request{OwnerUUID: "owner-uuid"}).Validate())
	})
}
