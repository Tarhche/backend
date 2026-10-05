package createSnapshot

import (
	"context"
	"encoding/json"
	"testing"
	"time"

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

	t.Run("takes a snapshot for whoever asks, and answers with it", func(t *testing.T) {
		t.Parallel()

		taking := workloadtest.Snapshot()
		taking.State = snapshot.Creating
		taking.Size = 0
		taking.Engine = ""
		taking.CompletedAt = time.Time{}

		var workload controlplane.MockClient
		workload.On("CreateSnapshot", mock.Anything, workloadtest.OwnerUUID, "vm-uuid", "before the upgrade").Once().Return(taking, nil)
		defer workload.AssertExpectations(t)

		response, err := useCase(&workload).Execute(context.Background(), &Request{
			VMUUID:    "vm-uuid",
			Name:      "before the upgrade",
			OwnerUUID: workloadtest.OwnerUUID,
		})
		require.NoError(t, err)

		presented, err := json.Marshal(response)
		require.NoError(t, err)

		// being taken until its archive is stored, so it has no size, no
		// engine and no moment it was done yet.
		assert.JSONEq(t, `{
			"uuid": "snapshot-uuid",
			"name": "before the upgrade",
			"owner_uuid": "owner-uuid",
			"owner": {"uuid": "owner-uuid", "name": "Mahdi", "username": "mahdi", "avatar": "avatar-uuid"},
			"vm_uuid": "vm-uuid",
			"vm_name": "docker-1",
			"kind": "docker",
			"image": "docker:29-dind",
			"disk": 21474836480,
			"size": 0,
			"state": "creating",
			"created_at": "2026-10-04T12:00:00Z"
		}`, string(presented))
	})

	t.Run("a snapshot needs a name", func(t *testing.T) {
		t.Parallel()

		var workload controlplane.MockClient

		response, err := useCase(&workload).Execute(context.Background(), &Request{VMUUID: "vm-uuid", OwnerUUID: workloadtest.OwnerUUID})
		require.NoError(t, err)

		assert.Equal(t, domain.ValidationErrors{"name": "this field is required"}, response.ValidationErrors)
		workload.AssertNotCalled(t, "CreateSnapshot", mock.Anything, mock.Anything, mock.Anything, mock.Anything)
	})

	for _, answer := range workloadtest.Answers() {
		t.Run(answer.Name, func(t *testing.T) {
			t.Parallel()

			var workload controlplane.MockClient
			workload.On("CreateSnapshot", mock.Anything, workloadtest.OwnerUUID, "vm-uuid", "nightly").Once().Return(snapshot.Snapshot{}, answer.Err)
			defer workload.AssertExpectations(t)

			response, err := useCase(&workload).Execute(context.Background(), &Request{
				VMUUID:    "vm-uuid",
				Name:      "nightly",
				OwnerUUID: workloadtest.OwnerUUID,
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
