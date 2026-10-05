package createNetwork

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
			daemon.On("CreateNetwork", mock.Anything, docker.NetworkSpec{Name: "backend", Driver: "bridge", Internal: true}).Once().Return(docker.Network{
				ID: "n1", Name: "backend", Driver: "bridge", Scope: "local", Internal: true, CreatedAt: workloadtest.At,
			}, nil)
			defer daemon.AssertExpectations(t)

			var workload controlplane.MockClient
			workload.On("Docker", scope.OwnerUUID, "vm-uuid").Once().Return(&daemon)
			defer workload.AssertExpectations(t)

			response, err := useCase(&workload).Execute(context.Background(), &Request{
				VMUUID:    "vm-uuid",
				Name:      "backend",
				Driver:    "bridge",
				Internal:  true,
				OwnerUUID: scope.OwnerUUID,
			})
			require.NoError(t, err)

			presented, err := json.Marshal(response.DockerNetwork)
			require.NoError(t, err)

			assert.JSONEq(t, `{"id": "n1", "name": "backend", "driver": "bridge", "scope": "local", "internal": true, "containers": [], "created_at": "2026-10-04T12:00:00Z"}`, string(presented))
		})
	}

	t.Run("a network needs a name docker takes, and no driver but bridge", func(t *testing.T) {
		t.Parallel()

		var workload controlplane.MockClient

		response, err := useCase(&workload).Execute(context.Background(), &Request{VMUUID: "vm-uuid", Name: "my network", Driver: "overlay"})
		require.NoError(t, err)

		assert.Equal(t, domain.ValidationErrors{
			"name":   "may hold only letters, digits, dots, dashes and underscores, and starts with a letter or a digit",
			"driver": "a network's driver can only be bridge",
		}, response.ValidationErrors)
		workload.AssertNotCalled(t, "Docker", mock.Anything, mock.Anything)
	})

	t.Run("and a name to begin with", func(t *testing.T) {
		t.Parallel()

		assert.Equal(t, domain.ValidationErrors{"name": "required_field"}, (&Request{}).Validate())
	})

	for _, answer := range workloadtest.Answers() {
		t.Run(answer.Name, func(t *testing.T) {
			t.Parallel()

			var daemon dockerMock.MockDaemon
			daemon.On("CreateNetwork", mock.Anything, docker.NetworkSpec{Name: "backend"}).Once().Return(docker.Network{}, answer.Err)
			defer daemon.AssertExpectations(t)

			var workload controlplane.MockClient
			workload.On("Docker", workloadtest.OwnerUUID, "vm-uuid").Once().Return(&daemon)
			defer workload.AssertExpectations(t)

			response, err := useCase(&workload).Execute(context.Background(), &Request{
				VMUUID:    "vm-uuid",
				Name:      "backend",
				OwnerUUID: workloadtest.OwnerUUID,
			})

			if answer.Failure != nil {
				assert.ErrorIs(t, err, answer.Failure)
				assert.Nil(t, response)

				return
			}

			require.NoError(t, err)
			assert.Equal(t, answer.Refused, response.ValidationErrors)
			assert.Nil(t, response.DockerNetwork)
		})
	}
}
