package stopusercontainer

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"

	"github.com/khanzadimahdi/testproject/domain"
	"github.com/khanzadimahdi/testproject/domain/runner/task"
	runnerMock "github.com/khanzadimahdi/testproject/infrastructure/repository/mocks/runner/manager"
)

func TestUseCase_Execute(t *testing.T) {
	t.Parallel()

	t.Run("stops a container the person owns", func(t *testing.T) {
		t.Parallel()

		var (
			runner runnerMock.MockClient

			r = Request{UUID: "container-uuid", OwnerUUID: "owner-uuid"}
		)

		runner.On("ContainerOf", mock.Anything, r.OwnerUUID, r.UUID).Once().Return(task.Task{UUID: r.UUID, OwnerUUID: r.OwnerUUID}, nil)
		runner.On("StopContainer", mock.Anything, r.UUID).Once().Return(nil)
		defer runner.AssertExpectations(t)

		assert.NoError(t, NewUseCase(&runner).Execute(context.Background(), &r))
	})

	t.Run("a container somebody else owns is not there to stop", func(t *testing.T) {
		t.Parallel()

		var (
			runner runnerMock.MockClient

			r = Request{UUID: "container-uuid", OwnerUUID: "owner-uuid"}
		)

		// asked for as this person's, a container that is not theirs is not
		// found — the same answer as one that does not exist.
		runner.On("ContainerOf", mock.Anything, r.OwnerUUID, r.UUID).Once().Return(task.Task{}, domain.ErrNotExists)
		defer runner.AssertExpectations(t)

		assert.ErrorIs(t, NewUseCase(&runner).Execute(context.Background(), &r), domain.ErrNotExists)

		// and nothing is asked of the runner about it.
		runner.AssertNotCalled(t, "StopContainer", mock.Anything, mock.Anything)
	})
}
