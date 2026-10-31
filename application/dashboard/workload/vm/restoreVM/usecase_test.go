package restoreVM

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/khanzadimahdi/testproject/application/dashboard/workload/workloadtest"
	"github.com/khanzadimahdi/testproject/domain"
	"github.com/khanzadimahdi/testproject/domain/workload/vm"
	"github.com/khanzadimahdi/testproject/infrastructure/repository/mocks/workload/controlplane"
)

func useCase(workload *controlplane.MockClient) *UseCase {
	return NewUseCase(workload, workloadtest.Validator(), workloadtest.Translator())
}

func TestUseCase_Execute(t *testing.T) {
	t.Parallel()

	for _, scope := range workloadtest.Scopes {
		t.Run(scope.Name, func(t *testing.T) {
			t.Parallel()

			var workload controlplane.MockClient
			workload.On("RestoreVM", mock.Anything, scope.OwnerUUID, "vm-uuid", "snapshot-uuid").Once().Return(nil)
			defer workload.AssertExpectations(t)

			response, err := useCase(&workload).Execute(context.Background(), &Request{
				UUID:         "vm-uuid",
				OwnerUUID:    scope.OwnerUUID,
				SnapshotUUID: "snapshot-uuid",
			})
			require.NoError(t, err)

			assert.Empty(t, response.ValidationErrors)
		})
	}

	t.Run("a restore needs a snapshot to restore from", func(t *testing.T) {
		t.Parallel()

		var workload controlplane.MockClient

		response, err := useCase(&workload).Execute(context.Background(), &Request{UUID: "vm-uuid"})
		require.NoError(t, err)

		assert.Equal(t, domain.ValidationErrors{"snapshot_uuid": "this field is required"}, response.ValidationErrors)
		workload.AssertNotCalled(t, "RestoreVM", mock.Anything, mock.Anything, mock.Anything, mock.Anything)
	})

	t.Run("a snapshot another engine took is said to be about the snapshot", func(t *testing.T) {
		t.Parallel()

		var workload controlplane.MockClient
		workload.On("RestoreVM", mock.Anything, "", "vm-uuid", "snapshot-uuid").Once().Return(vm.ErrEngineMismatch)
		defer workload.AssertExpectations(t)

		response, err := useCase(&workload).Execute(context.Background(), &Request{UUID: "vm-uuid", SnapshotUUID: "snapshot-uuid"})
		require.NoError(t, err)

		assert.Equal(t, domain.ValidationErrors{
			"snapshot_uuid": "the snapshot was taken by another engine and cannot be restored here",
		}, response.ValidationErrors)
	})

	for _, answer := range workloadtest.Answers() {
		t.Run(answer.Name, func(t *testing.T) {
			t.Parallel()

			var workload controlplane.MockClient
			workload.On("RestoreVM", mock.Anything, workloadtest.OwnerUUID, "vm-uuid", "snapshot-uuid").Once().Return(answer.Err)
			defer workload.AssertExpectations(t)

			response, err := useCase(&workload).Execute(context.Background(), &Request{
				UUID:         "vm-uuid",
				OwnerUUID:    workloadtest.OwnerUUID,
				SnapshotUUID: "snapshot-uuid",
			})

			if answer.Failure != nil {
				assert.ErrorIs(t, err, answer.Failure)
				assert.Nil(t, response)

				return
			}

			require.NoError(t, err)
			assert.Equal(t, answer.Refused, response.ValidationErrors)
		})
	}
}
