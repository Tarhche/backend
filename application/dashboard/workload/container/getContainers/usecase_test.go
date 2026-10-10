package getContainers

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/khanzadimahdi/testproject/application/dashboard/workload/workloadtest"
	workloadControlPlane "github.com/khanzadimahdi/testproject/domain/workload/controlplane"
	"github.com/khanzadimahdi/testproject/domain/workload/stack"
	"github.com/khanzadimahdi/testproject/infrastructure/repository/mocks/workload/controlplane"
)

func TestUseCase_Execute(t *testing.T) {
	t.Parallel()

	for _, scope := range workloadtest.Scopes {
		t.Run(scope.Name, func(t *testing.T) {
			t.Parallel()

			var workload controlplane.MockClient
			workload.On("Containers", mock.Anything, scope.OwnerUUID, "").Once().Return([]workloadControlPlane.VMContainer{
				{Container: workloadtest.Container(), VMUUID: "vm-uuid", VMName: "docker-1"},
			}, nil)
			workload.On("Stacks", mock.Anything, scope.OwnerUUID, "", uint(1)).Once().Return(workloadControlPlane.Page[stack.Stack]{
				Items:       []stack.Stack{workloadtest.Stack()},
				CurrentPage: 1,
				TotalPages:  1,
			}, nil)
			defer workload.AssertExpectations(t)

			response, err := NewUseCase(&workload, workloadtest.Translator()).Execute(context.Background(), &Request{OwnerUUID: scope.OwnerUUID})
			require.NoError(t, err)

			presented, err := json.Marshal(response)
			require.NoError(t, err)

			// each says the VM it is in, and the stack that deployed it.
			var items []map[string]any
			require.NoError(t, json.Unmarshal(presented, &struct {
				Items *[]map[string]any `json:"items"`
			}{&items}))

			require.Len(t, items, 1)
			assert.Equal(t, "vm-uuid", items[0]["vm_uuid"])
			assert.Equal(t, "docker-1", items[0]["vm_name"])
			assert.Equal(t, "shop-abcde", items[0]["stack"])
			assert.Equal(t, "stack-uuid", items[0]["stack_uuid"])
		})
	}

	t.Run("one VM's, and the stacks of that VM only", func(t *testing.T) {
		t.Parallel()

		var workload controlplane.MockClient
		workload.On("Containers", mock.Anything, workloadtest.OwnerUUID, "vm-uuid").Once().Return(nil, nil)
		workload.On("Stacks", mock.Anything, workloadtest.OwnerUUID, "vm-uuid", uint(1)).Once().Return(workloadControlPlane.Page[stack.Stack]{}, nil)
		defer workload.AssertExpectations(t)

		response, err := NewUseCase(&workload, workloadtest.Translator()).Execute(context.Background(), &Request{
			VMUUID:    "vm-uuid",
			OwnerUUID: workloadtest.OwnerUUID,
		})
		require.NoError(t, err)

		presented, err := json.Marshal(response)
		require.NoError(t, err)

		assert.JSONEq(t, `{"items": []}`, string(presented))
	})

	for _, answer := range workloadtest.Answers() {
		t.Run(answer.Name, func(t *testing.T) {
			t.Parallel()

			var workload controlplane.MockClient
			workload.On("Containers", mock.Anything, workloadtest.OwnerUUID, "").Once().Return(nil, answer.Err)
			defer workload.AssertExpectations(t)

			response, err := NewUseCase(&workload, workloadtest.Translator()).Execute(context.Background(), &Request{OwnerUUID: workloadtest.OwnerUUID})

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
