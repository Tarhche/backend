package stopusertask

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"

	"github.com/khanzadimahdi/testproject/domain"
	"github.com/khanzadimahdi/testproject/domain/workload/task"
	workloadMock "github.com/khanzadimahdi/testproject/infrastructure/repository/mocks/workload/controlplane"
)

func TestUseCase_Execute(t *testing.T) {
	t.Parallel()

	t.Run("stops a task the person owns", func(t *testing.T) {
		t.Parallel()

		var (
			workload workloadMock.MockClient

			r = Request{UUID: "task-uuid", OwnerUUID: "owner-uuid"}
		)

		workload.On("TaskOf", mock.Anything, r.OwnerUUID, r.UUID).Once().Return(task.Task{UUID: r.UUID, OwnerUUID: r.OwnerUUID}, nil)
		workload.On("StopTask", mock.Anything, r.UUID).Once().Return(nil)
		defer workload.AssertExpectations(t)

		assert.NoError(t, NewUseCase(&workload).Execute(context.Background(), &r))
	})

	t.Run("a task somebody else owns is not there to stop", func(t *testing.T) {
		t.Parallel()

		var (
			workload workloadMock.MockClient

			r = Request{UUID: "task-uuid", OwnerUUID: "owner-uuid"}
		)

		// asked for as this person's, a task that is not theirs is not
		// found — the same answer as one that does not exist.
		workload.On("TaskOf", mock.Anything, r.OwnerUUID, r.UUID).Once().Return(task.Task{}, domain.ErrNotExists)
		defer workload.AssertExpectations(t)

		assert.ErrorIs(t, NewUseCase(&workload).Execute(context.Background(), &r), domain.ErrNotExists)

		// and nothing is asked of the workload about it.
		workload.AssertNotCalled(t, "StopTask", mock.Anything, mock.Anything)
	})
}
