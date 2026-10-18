package getSnapshot

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/khanzadimahdi/testproject/application/dashboard/workload/workloadtest"
	"github.com/khanzadimahdi/testproject/domain/workload/snapshot"
	"github.com/khanzadimahdi/testproject/infrastructure/repository/mocks/workload/controlplane"
)

func TestUseCase_Execute(t *testing.T) {
	t.Parallel()

	for _, scope := range workloadtest.Scopes {
		t.Run(scope.Name, func(t *testing.T) {
			t.Parallel()

			var workload controlplane.MockClient
			workload.On("Snapshot", mock.Anything, scope.OwnerUUID, "snapshot-uuid").Once().Return(workloadtest.Snapshot(), nil)
			defer workload.AssertExpectations(t)

			response, err := NewUseCase(&workload, workloadtest.Owners()).Execute(context.Background(), &Request{
				UUID:      "snapshot-uuid",
				OwnerUUID: scope.OwnerUUID,
			})
			require.NoError(t, err)

			presented, err := json.Marshal(response)
			require.NoError(t, err)

			assert.JSONEq(t, workloadtest.SnapshotJSON, string(presented))
		})
	}

	for _, answer := range workloadtest.Failures() {
		t.Run(answer.Name, func(t *testing.T) {
			t.Parallel()

			var workload controlplane.MockClient
			workload.On("Snapshot", mock.Anything, workloadtest.OwnerUUID, "snapshot-uuid").Once().Return(snapshot.Snapshot{}, answer.Err)
			defer workload.AssertExpectations(t)

			response, err := NewUseCase(&workload, workloadtest.Owners()).Execute(context.Background(), &Request{
				UUID:      "snapshot-uuid",
				OwnerUUID: workloadtest.OwnerUUID,
			})

			assert.ErrorIs(t, err, answer.Failure)
			assert.Nil(t, response)
		})
	}
}
