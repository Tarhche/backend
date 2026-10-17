package getStack

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
	"github.com/khanzadimahdi/testproject/infrastructure/repository/mocks/workload/controlplane"
)

func TestUseCase_Execute(t *testing.T) {
	t.Parallel()

	for _, scope := range workloadtest.Scopes {
		t.Run(scope.Name, func(t *testing.T) {
			t.Parallel()

			var workload controlplane.MockClient
			workload.On("Stack", mock.Anything, scope.OwnerUUID, "stack-uuid").Once().Return(workloadControlPlane.StackDetail{
				Stack:      workloadtest.Stack(),
				Containers: []docker.Container{workloadtest.Container()},
			}, nil)
			defer workload.AssertExpectations(t)

			response, err := NewUseCase(&workload, workloadtest.Owners()).Execute(context.Background(), &Request{
				UUID:      "stack-uuid",
				OwnerUUID: scope.OwnerUUID,
			})
			require.NoError(t, err)

			presented, err := json.Marshal(response)
			require.NoError(t, err)

			var detail map[string]any
			require.NoError(t, json.Unmarshal(presented, &detail))

			var stack map[string]any
			require.NoError(t, json.Unmarshal([]byte(workloadtest.StackJSON), &stack))

			for field, value := range stack {
				assert.Equal(t, value, detail[field], field)
			}

			containers, ok := detail["containers"].([]any)
			require.True(t, ok)
			require.Len(t, containers, 1)
			assert.Equal(t, "stack-uuid", containers[0].(map[string]any)["stack_uuid"])
			assert.NotContains(t, detail, "note")
		})
	}

	t.Run("a stack whose VM is not running has no containers to show, and says why", func(t *testing.T) {
		t.Parallel()

		var workload controlplane.MockClient
		workload.On("Stack", mock.Anything, "", "stack-uuid").Once().Return(workloadControlPlane.StackDetail{
			Stack:        workloadtest.Stack(),
			VMNotRunning: true,
		}, nil)
		defer workload.AssertExpectations(t)

		response, err := NewUseCase(&workload, workloadtest.Owners()).Execute(context.Background(), &Request{UUID: "stack-uuid"})
		require.NoError(t, err)

		presented, err := json.Marshal(response)
		require.NoError(t, err)

		var detail map[string]any
		require.NoError(t, json.Unmarshal(presented, &detail))

		assert.Equal(t, []any{}, detail["containers"])
		assert.Equal(t, "vm_not_running", detail["note"])
	})

	for _, answer := range workloadtest.Failures() {
		t.Run(answer.Name, func(t *testing.T) {
			t.Parallel()

			var workload controlplane.MockClient
			workload.On("Stack", mock.Anything, workloadtest.OwnerUUID, "stack-uuid").Once().Return(workloadControlPlane.StackDetail{}, answer.Err)
			defer workload.AssertExpectations(t)

			response, err := NewUseCase(&workload, workloadtest.Owners()).Execute(context.Background(), &Request{
				UUID:      "stack-uuid",
				OwnerUUID: workloadtest.OwnerUUID,
			})

			assert.ErrorIs(t, err, answer.Failure)
			assert.Nil(t, response)
		})
	}
}
