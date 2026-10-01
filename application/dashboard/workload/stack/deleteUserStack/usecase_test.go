package deleteuserstack

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"

	"github.com/khanzadimahdi/testproject/domain"
	workloadControlPlane "github.com/khanzadimahdi/testproject/domain/workload/controlplane"
	workloadMock "github.com/khanzadimahdi/testproject/infrastructure/repository/mocks/workload/controlplane"
)

func TestUseCase_Execute(t *testing.T) {
	t.Parallel()

	t.Run("takes away a stack the person owns", func(t *testing.T) {
		t.Parallel()

		var (
			workload workloadMock.MockClient

			r = Request{UUID: "stack-uuid", OwnerUUID: "owner-uuid"}
		)

		workload.On("StackOf", mock.Anything, r.OwnerUUID, r.UUID).Once().Return(workloadControlPlane.Stack{UUID: r.UUID, OwnerUUID: r.OwnerUUID}, nil)
		workload.On("DeleteStack", mock.Anything, r.UUID).Once().Return(nil)
		defer workload.AssertExpectations(t)

		assert.NoError(t, NewUseCase(&workload).Execute(context.Background(), &r))
	})

	t.Run("a stack somebody else owns is not there to delete", func(t *testing.T) {
		t.Parallel()

		var (
			workload workloadMock.MockClient

			r = Request{UUID: "stack-uuid", OwnerUUID: "owner-uuid"}
		)

		// asked for as this person's, a stack that is not theirs is not
		// found — the same answer as one that does not exist.
		workload.On("StackOf", mock.Anything, r.OwnerUUID, r.UUID).Once().Return(workloadControlPlane.Stack{}, domain.ErrNotExists)
		defer workload.AssertExpectations(t)

		assert.ErrorIs(t, NewUseCase(&workload).Execute(context.Background(), &r), domain.ErrNotExists)

		// and nothing is asked of the workload about it.
		workload.AssertNotCalled(t, "DeleteStack", mock.Anything, mock.Anything)
	})
}
