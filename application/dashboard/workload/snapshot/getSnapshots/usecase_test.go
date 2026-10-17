package getSnapshots

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/khanzadimahdi/testproject/application/dashboard/workload/workloadtest"
	workloadControlPlane "github.com/khanzadimahdi/testproject/domain/workload/controlplane"
	"github.com/khanzadimahdi/testproject/domain/workload/snapshot"
	"github.com/khanzadimahdi/testproject/infrastructure/repository/mocks/workload/controlplane"
)

func TestUseCase_Execute(t *testing.T) {
	t.Parallel()

	for _, scope := range workloadtest.Scopes {
		t.Run(scope.Name, func(t *testing.T) {
			t.Parallel()

			var workload controlplane.MockClient
			workload.On("Snapshots", mock.Anything, scope.OwnerUUID, "vm-uuid", uint(2)).Once().Return(workloadControlPlane.Page[snapshot.Snapshot]{
				Items:       []snapshot.Snapshot{workloadtest.Snapshot()},
				TotalPages:  2,
				CurrentPage: 2,
			}, nil)
			defer workload.AssertExpectations(t)

			response, err := NewUseCase(&workload, workloadtest.Owners()).Execute(context.Background(), &Request{
				Page:      2,
				VMUUID:    "vm-uuid",
				OwnerUUID: scope.OwnerUUID,
			})
			require.NoError(t, err)

			presented, err := json.Marshal(response)
			require.NoError(t, err)

			assert.JSONEq(t, `{"items": [`+workloadtest.SnapshotJSON+`], "pagination": {"total_pages": 2, "current_page": 2}}`, string(presented))
		})
	}

	t.Run("the first page of every VM's when the request says neither", func(t *testing.T) {
		t.Parallel()

		var workload controlplane.MockClient
		workload.On("Snapshots", mock.Anything, "", "", uint(1)).Once().Return(workloadControlPlane.Page[snapshot.Snapshot]{CurrentPage: 1}, nil)
		defer workload.AssertExpectations(t)

		response, err := NewUseCase(&workload, workloadtest.Owners()).Execute(context.Background(), &Request{})
		require.NoError(t, err)

		assert.Empty(t, response.Items)
		assert.NotNil(t, response.Items)
	})

	for _, answer := range workloadtest.Failures() {
		t.Run(answer.Name, func(t *testing.T) {
			t.Parallel()

			var workload controlplane.MockClient
			workload.On("Snapshots", mock.Anything, workloadtest.OwnerUUID, "", uint(1)).Once().Return(workloadControlPlane.Page[snapshot.Snapshot]{}, answer.Err)
			defer workload.AssertExpectations(t)

			response, err := NewUseCase(&workload, workloadtest.Owners()).Execute(context.Background(), &Request{OwnerUUID: workloadtest.OwnerUUID})

			assert.ErrorIs(t, err, answer.Failure)
			assert.Nil(t, response)
		})
	}
}
