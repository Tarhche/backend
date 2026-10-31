package renameSnapshot

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/khanzadimahdi/testproject/application/dashboard/workload/workloadtest"
	"github.com/khanzadimahdi/testproject/domain"
	"github.com/khanzadimahdi/testproject/domain/workload/snapshot"
	"github.com/khanzadimahdi/testproject/infrastructure/repository/mocks/workload/controlplane"
)

func useCase(workload *controlplane.MockClient) *UseCase {
	return NewUseCase(workload, workloadtest.Validator(), workloadtest.Translator(), workloadtest.Owners())
}

func TestUseCase_Execute(t *testing.T) {
	t.Parallel()

	for _, scope := range workloadtest.Scopes {
		t.Run(scope.Name, func(t *testing.T) {
			t.Parallel()

			var workload controlplane.MockClient
			workload.On("RenameSnapshot", mock.Anything, scope.OwnerUUID, "snapshot-uuid", "before the upgrade").Once().Return(workloadtest.Snapshot(), nil)
			defer workload.AssertExpectations(t)

			response, err := useCase(&workload).Execute(context.Background(), &Request{
				UUID:      "snapshot-uuid",
				OwnerUUID: scope.OwnerUUID,
				Name:      "before the upgrade",
			})
			require.NoError(t, err)

			presented, err := json.Marshal(response)
			require.NoError(t, err)

			assert.JSONEq(t, workloadtest.SnapshotJSON, string(presented))
		})
	}

	t.Run("a snapshot is not renamed to nothing", func(t *testing.T) {
		t.Parallel()

		var workload controlplane.MockClient

		response, err := useCase(&workload).Execute(context.Background(), &Request{UUID: "snapshot-uuid", Name: "  "})
		require.NoError(t, err)

		assert.Equal(t, domain.ValidationErrors{"name": "this field is required"}, response.ValidationErrors)
		workload.AssertNotCalled(t, "RenameSnapshot", mock.Anything, mock.Anything, mock.Anything, mock.Anything)
	})

	for _, answer := range workloadtest.Answers() {
		t.Run(answer.Name, func(t *testing.T) {
			t.Parallel()

			var workload controlplane.MockClient
			workload.On("RenameSnapshot", mock.Anything, workloadtest.OwnerUUID, "snapshot-uuid", "nightly").Once().Return(snapshot.Snapshot{}, answer.Err)
			defer workload.AssertExpectations(t)

			response, err := useCase(&workload).Execute(context.Background(), &Request{
				UUID:      "snapshot-uuid",
				OwnerUUID: workloadtest.OwnerUUID,
				Name:      "nightly",
			})

			if answer.Failure != nil {
				assert.ErrorIs(t, err, answer.Failure)
				assert.Nil(t, response)

				return
			}

			require.NoError(t, err)
			assert.Equal(t, answer.Refused, response.ValidationErrors)
			assert.Nil(t, response.Snapshot)
		})
	}
}
