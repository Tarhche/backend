package getVMContainers

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/khanzadimahdi/testproject/application/dashboard/workload/workloadtest"
	workloadControlPlane "github.com/khanzadimahdi/testproject/domain/workload/controlplane"
	"github.com/khanzadimahdi/testproject/domain/workload/docker"
	"github.com/khanzadimahdi/testproject/domain/workload/stack"
	"github.com/khanzadimahdi/testproject/infrastructure/repository/mocks/workload/controlplane"
	dockerMock "github.com/khanzadimahdi/testproject/infrastructure/repository/mocks/workload/docker"
)

func TestUseCase_Execute(t *testing.T) {
	t.Parallel()

	for _, scope := range workloadtest.Scopes {
		t.Run(scope.Name, func(t *testing.T) {
			t.Parallel()

			// stopped ones too: a container that exited is one somebody may
			// want to start again, or remove.
			var daemon dockerMock.MockDaemon
			daemon.On("Containers", mock.Anything, docker.ContainerFilter{All: true}).Once().Return([]docker.Container{workloadtest.Container()}, nil)
			defer daemon.AssertExpectations(t)

			var workload controlplane.MockClient
			workload.On("Docker", scope.OwnerUUID, "vm-uuid").Once().Return(&daemon)
			workload.On("Stacks", mock.Anything, scope.OwnerUUID, "vm-uuid", uint(1)).Once().Return(workloadControlPlane.Page[stack.Stack]{
				Items:       []stack.Stack{workloadtest.Stack()},
				CurrentPage: 1,
				TotalPages:  1,
			}, nil)
			defer workload.AssertExpectations(t)

			response, err := NewUseCase(&workload, workloadtest.Translator()).Execute(context.Background(), &Request{
				VMUUID:    "vm-uuid",
				OwnerUUID: scope.OwnerUUID,
			})
			require.NoError(t, err)

			presented, err := json.Marshal(response)
			require.NoError(t, err)

			var listing struct {
				Items []map[string]any `json:"items"`
			}
			require.NoError(t, json.Unmarshal(presented, &listing))

			require.Len(t, listing.Items, 1)
			assert.Equal(t, "c0ffee", listing.Items[0]["id"])
			assert.Equal(t, "stack-uuid", listing.Items[0]["stack_uuid"])
		})
	}

	t.Run("the stacks failing to be read leaves the containers unlinked, not unlisted", func(t *testing.T) {
		t.Parallel()

		var daemon dockerMock.MockDaemon
		daemon.On("Containers", mock.Anything, docker.ContainerFilter{All: true}).Once().Return([]docker.Container{workloadtest.Container()}, nil)
		defer daemon.AssertExpectations(t)

		var workload controlplane.MockClient
		workload.On("Docker", "", "vm-uuid").Once().Return(&daemon)
		workload.On("Stacks", mock.Anything, "", "vm-uuid", uint(1)).Once().Return(workloadControlPlane.Page[stack.Stack]{}, workloadtest.ErrUnreachable)
		defer workload.AssertExpectations(t)

		response, err := NewUseCase(&workload, workloadtest.Translator()).Execute(context.Background(), &Request{VMUUID: "vm-uuid"})
		require.NoError(t, err)

		require.Len(t, response.Items, 1)
		assert.Empty(t, response.Items[0].StackUUID)
		assert.Equal(t, "shop-abcde", response.Items[0].Stack)
	})

	for _, answer := range workloadtest.Answers() {
		t.Run(answer.Name, func(t *testing.T) {
			t.Parallel()

			var daemon dockerMock.MockDaemon
			daemon.On("Containers", mock.Anything, docker.ContainerFilter{All: true}).Once().Return(nil, answer.Err)
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
