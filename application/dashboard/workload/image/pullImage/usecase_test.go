package pullImage

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/khanzadimahdi/testproject/application/dashboard/workload/workloadtest"
	"github.com/khanzadimahdi/testproject/domain"
	"github.com/khanzadimahdi/testproject/domain/workload/docker"
	"github.com/khanzadimahdi/testproject/infrastructure/repository/mocks/workload/controlplane"
	dockerMock "github.com/khanzadimahdi/testproject/infrastructure/repository/mocks/workload/docker"
)

func useCase(workload *controlplane.MockClient) *UseCase {
	return NewUseCase(workload, workloadtest.Validator(), workloadtest.Translator())
}

func TestUseCase_Execute(t *testing.T) {
	t.Parallel()

	for _, scope := range workloadtest.Scopes {
		t.Run(scope.Name, func(t *testing.T) {
			t.Parallel()

			var daemon dockerMock.MockDaemon
			daemon.On("PullImage", mock.Anything, "nginx:1.27").Once().Return(docker.Image{
				ID: "sha256:1", Tags: []string{"nginx:1.27"}, Size: 1 << 20, CreatedAt: workloadtest.At,
			}, nil)
			defer daemon.AssertExpectations(t)

			var workload controlplane.MockClient
			workload.On("Docker", scope.OwnerUUID, "vm-uuid").Once().Return(&daemon)
			defer workload.AssertExpectations(t)

			response, err := useCase(&workload).Execute(context.Background(), &Request{
				VMUUID:    "vm-uuid",
				Reference: "nginx:1.27",
				OwnerUUID: scope.OwnerUUID,
			})
			require.NoError(t, err)

			presented, err := json.Marshal(response.Image)
			require.NoError(t, err)

			assert.JSONEq(t, `{"id": "sha256:1", "tags": ["nginx:1.27"], "size": 1048576, "created_at": "2026-10-04T12:00:00Z", "in_use": false}`, string(presented))
		})
	}

	t.Run("a reference with a space in it is no reference, and nothing is pulled", func(t *testing.T) {
		t.Parallel()

		var workload controlplane.MockClient

		response, err := useCase(&workload).Execute(context.Background(), &Request{VMUUID: "vm-uuid", Reference: "nginx latest"})
		require.NoError(t, err)

		assert.Equal(t, domain.ValidationErrors{
			"reference": "an image is named by a reference with no spaces in it, such as nginx:1.27",
		}, response.ValidationErrors)
		workload.AssertNotCalled(t, "Docker", mock.Anything, mock.Anything)
	})

	for _, answer := range workloadtest.Answers() {
		t.Run(answer.Name, func(t *testing.T) {
			t.Parallel()

			var daemon dockerMock.MockDaemon
			daemon.On("PullImage", mock.Anything, "nginx").Once().Return(docker.Image{}, answer.Err)
			defer daemon.AssertExpectations(t)

			var workload controlplane.MockClient
			workload.On("Docker", workloadtest.OwnerUUID, "vm-uuid").Once().Return(&daemon)
			defer workload.AssertExpectations(t)

			response, err := useCase(&workload).Execute(context.Background(), &Request{
				VMUUID:    "vm-uuid",
				Reference: "nginx",
				OwnerUUID: workloadtest.OwnerUUID,
			})

			if answer.Failure != nil {
				assert.ErrorIs(t, err, answer.Failure)
				assert.Nil(t, response)

				return
			}

			require.NoError(t, err)
			assert.Equal(t, answer.Refused, response.ValidationErrors)
			assert.Nil(t, response.Image)
		})
	}
}
