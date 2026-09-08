package deleteuserstack

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"

	"github.com/khanzadimahdi/testproject/domain"
	runnerManager "github.com/khanzadimahdi/testproject/domain/runner/manager"
	"github.com/khanzadimahdi/testproject/domain/runner/stack"
	runnerMock "github.com/khanzadimahdi/testproject/infrastructure/repository/mocks/runner/manager"
)

func TestUseCase_Execute(t *testing.T) {
	t.Parallel()

	t.Run("takes away a stack the person owns", func(t *testing.T) {
		t.Parallel()

		var (
			runner runnerMock.MockClient

			r = Request{UUID: "stack-uuid", OwnerUUID: "owner-uuid"}
		)

		runner.On("StackOf", mock.Anything, r.OwnerUUID, r.UUID).Once().Return(runnerManager.Stack{Stack: stack.Stack{UUID: r.UUID, OwnerUUID: r.OwnerUUID}}, nil)
		runner.On("DeleteStack", mock.Anything, r.UUID).Once().Return(nil)
		defer runner.AssertExpectations(t)

		assert.NoError(t, NewUseCase(&runner).Execute(context.Background(), &r))
	})

	t.Run("a stack somebody else owns is not there to delete", func(t *testing.T) {
		t.Parallel()

		var (
			runner runnerMock.MockClient

			r = Request{UUID: "stack-uuid", OwnerUUID: "owner-uuid"}
		)

		// asked for as this person's, a stack that is not theirs is not
		// found — the same answer as one that does not exist.
		runner.On("StackOf", mock.Anything, r.OwnerUUID, r.UUID).Once().Return(runnerManager.Stack{}, domain.ErrNotExists)
		defer runner.AssertExpectations(t)

		assert.ErrorIs(t, NewUseCase(&runner).Execute(context.Background(), &r), domain.ErrNotExists)

		// and nothing is asked of the runner about it.
		runner.AssertNotCalled(t, "DeleteStack", mock.Anything, mock.Anything)
	})
}
