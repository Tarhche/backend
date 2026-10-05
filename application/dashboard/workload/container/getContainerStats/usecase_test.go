package getContainerStats

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
			daemon.On("ContainerStats", mock.Anything, "c0ffee").Once().Return(docker.Stats{
				CPUPercent:  7.5,
				MemoryUsed:  64 << 20,
				MemoryLimit: 512 << 20,
				NetworkRx:   10,
				NetworkTx:   20,
				BlockRead:   30,
				BlockWrite:  40,
				PIDs:        3,
				SampledAt:   workloadtest.At,
			}, nil)
			defer daemon.AssertExpectations(t)

			var workload controlplane.MockClient
			workload.On("Docker", scope.OwnerUUID, "vm-uuid").Once().Return(&daemon)
			defer workload.AssertExpectations(t)

			response, err := NewUseCase(&workload, workloadtest.Translator()).Execute(context.Background(), &Request{
				VMUUID:    "vm-uuid",
				ID:        "c0ffee",
				OwnerUUID: scope.OwnerUUID,
			})
			require.NoError(t, err)

			presented, err := json.Marshal(response.ContainerStats)
			require.NoError(t, err)

			assert.JSONEq(t, `{
				"cpu_percent": 7.5,
				"memory_used": 67108864,
				"memory_limit": 536870912,
				"network_rx": 10,
				"network_tx": 20,
				"block_read": 30,
				"block_write": 40,
				"pids": 3,
				"sampled_at": "2026-10-04T12:00:00Z"
			}`, string(presented))
		})
	}

	for _, answer := range workloadtest.Answers() {
		t.Run(answer.Name, func(t *testing.T) {
			t.Parallel()

			var daemon dockerMock.MockDaemon
			daemon.On("ContainerStats", mock.Anything, "c0ffee").Once().Return(docker.Stats{}, answer.Err)
			defer daemon.AssertExpectations(t)

			var workload controlplane.MockClient
			workload.On("Docker", workloadtest.OwnerUUID, "vm-uuid").Once().Return(&daemon)
			defer workload.AssertExpectations(t)

			response, err := NewUseCase(&workload, workloadtest.Translator()).Execute(context.Background(), &Request{
				VMUUID:    "vm-uuid",
				ID:        "c0ffee",
				OwnerUUID: workloadtest.OwnerUUID,
			})

			if answer.Failure != nil {
				assert.ErrorIs(t, err, answer.Failure)
				assert.Nil(t, response)

				return
			}

			require.NoError(t, err)
			assert.Equal(t, answer.Refused, response.ValidationErrors)
			assert.Nil(t, response.ContainerStats)
		})
	}
}
