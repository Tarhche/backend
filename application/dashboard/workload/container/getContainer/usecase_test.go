package getContainer

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

			var daemon dockerMock.MockDaemon
			daemon.On("Container", mock.Anything, "c0ffee").Once().Return(workloadtest.Container(), nil)
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
				ID:        "c0ffee",
				OwnerUUID: scope.OwnerUUID,
			})
			require.NoError(t, err)

			presented, err := json.Marshal(response.Container)
			require.NoError(t, err)

			var shown map[string]any
			require.NoError(t, json.Unmarshal([]byte(workloadtest.ContainerJSON), &shown))
			shown["stack_uuid"] = "stack-uuid"

			expected, err := json.Marshal(shown)
			require.NoError(t, err)

			assert.JSONEq(t, string(expected), string(presented))
		})
	}

	t.Run("a container no stack deployed asks nothing about stacks", func(t *testing.T) {
		t.Parallel()

		var daemon dockerMock.MockDaemon
		daemon.On("Container", mock.Anything, "web").Once().Return(docker.Container{ID: "c0ffee", Name: "web"}, nil)
		defer daemon.AssertExpectations(t)

		var workload controlplane.MockClient
		workload.On("Docker", "", "vm-uuid").Once().Return(&daemon)
		defer workload.AssertExpectations(t)

		response, err := NewUseCase(&workload, workloadtest.Translator()).Execute(context.Background(), &Request{VMUUID: "vm-uuid", ID: "web"})
		require.NoError(t, err)

		assert.Empty(t, response.Container.StackUUID)
		workload.AssertNotCalled(t, "Stacks", mock.Anything, mock.Anything, mock.Anything, mock.Anything)
	})

	for _, answer := range workloadtest.Answers() {
		t.Run(answer.Name, func(t *testing.T) {
			t.Parallel()

			var daemon dockerMock.MockDaemon
			daemon.On("Container", mock.Anything, "c0ffee").Once().Return(docker.Container{}, answer.Err)
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
			assert.Nil(t, response.Container)
		})
	}
}
