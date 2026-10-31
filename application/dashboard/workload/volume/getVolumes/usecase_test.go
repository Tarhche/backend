package getVolumes

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/khanzadimahdi/testproject/application/dashboard/workload/workloadtest"
	"github.com/khanzadimahdi/testproject/domain/workload/docker"
	"github.com/khanzadimahdi/testproject/infrastructure/repository/mocks/workload/controlplane"
	dockerMock "github.com/khanzadimahdi/testproject/infrastructure/repository/mocks/workload/docker"
)

func TestUseCase_Execute(t *testing.T) {
	t.Parallel()

	for _, scope := range workloadtest.Scopes {
		t.Run(scope.Name, func(t *testing.T) {
			t.Parallel()

			var daemon dockerMock.MockDaemon
			daemon.On("Volumes", mock.Anything).Once().Return([]docker.Volume{{Name: "data", Driver: "local", Mountpoint: "/var/lib/docker/volumes/data/_data", CreatedAt: workloadtest.At}}, nil)
			defer daemon.AssertExpectations(t)

			var workload controlplane.MockClient
			workload.On("Docker", scope.OwnerUUID, "vm-uuid").Once().Return(&daemon)
			defer workload.AssertExpectations(t)

			response, err := NewUseCase(&workload, workloadtest.Translator()).Execute(context.Background(), &Request{
				VMUUID:    "vm-uuid",
				OwnerUUID: scope.OwnerUUID,
			})
			require.NoError(t, err)

			presented, err := json.Marshal(response)
			require.NoError(t, err)

			assert.JSONEq(t, `{"items": [{"name": "data", "driver": "local", "mountpoint": "/var/lib/docker/volumes/data/_data", "in_use": false, "created_at": "2026-10-04T12:00:00Z"}]}`, string(presented))
		})
	}

	t.Run("a VM with none has an empty list, not null", func(t *testing.T) {
		t.Parallel()

		var daemon dockerMock.MockDaemon
		daemon.On("Volumes", mock.Anything).Once().Return(nil, nil)
		defer daemon.AssertExpectations(t)

		var workload controlplane.MockClient
		workload.On("Docker", "", "vm-uuid").Once().Return(&daemon)
		defer workload.AssertExpectations(t)

		response, err := NewUseCase(&workload, workloadtest.Translator()).Execute(context.Background(), &Request{VMUUID: "vm-uuid"})
		require.NoError(t, err)

		presented, err := json.Marshal(response)
		require.NoError(t, err)

		assert.JSONEq(t, `{"items": []}`, string(presented))
	})

	for _, answer := range workloadtest.Answers() {
		t.Run(answer.Name, func(t *testing.T) {
			t.Parallel()

			var daemon dockerMock.MockDaemon
			daemon.On("Volumes", mock.Anything).Once().Return(nil, answer.Err)
			defer daemon.AssertExpectations(t)

			var workload controlplane.MockClient
			workload.On("Docker", workloadtest.OwnerUUID, "vm-uuid").Once().Return(&daemon)
			defer workload.AssertExpectations(t)

			response, err := NewUseCase(&workload, workloadtest.Translator()).Execute(context.Background(), &Request{
				VMUUID:    "vm-uuid",
				OwnerUUID: workloadtest.OwnerUUID,
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
