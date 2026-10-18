package getVMDependents

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/khanzadimahdi/testproject/application/dashboard/workload/presenter"
	"github.com/khanzadimahdi/testproject/application/dashboard/workload/workloadtest"
	"github.com/khanzadimahdi/testproject/infrastructure/repository/mocks/workload/controlplane"
	workloadControlPlane "github.com/khanzadimahdi/testproject/infrastructure/workload/controlplane"
)

func TestUseCase_Execute(t *testing.T) {
	t.Parallel()

	for _, scope := range workloadtest.Scopes {
		t.Run(scope.Name, func(t *testing.T) {
			t.Parallel()

			var workload controlplane.MockClient
			workload.On("VMDependents", mock.Anything, scope.OwnerUUID, "vm-uuid").Once().Return(workloadControlPlane.Deleted{
				Dependents: []workloadControlPlane.Dependent{{Kind: "stack", UUID: "stack-uuid", Name: "shop", Recreates: true}},
				Recreated:  true,
			}, nil)
			defer workload.AssertExpectations(t)

			response, err := NewUseCase(&workload, workloadtest.Translator()).Execute(context.Background(), &Request{UUID: "vm-uuid", OwnerUUID: scope.OwnerUUID})
			require.NoError(t, err)

			assert.Empty(t, response.ValidationErrors)
			assert.Equal(t, []presenter.Dependent{{Kind: "stack", UUID: "stack-uuid", Name: "shop", Recreates: true}}, response.Dependents, "the stack in it, which makes it again")
			assert.True(t, response.Recreated)
		})
	}

	t.Run("one nothing needs is answered with a list of none, and nothing else", func(t *testing.T) {
		t.Parallel()

		var workload controlplane.MockClient
		workload.On("VMDependents", mock.Anything, workloadtest.OwnerUUID, "vm-uuid").Once().Return(workloadControlPlane.Deleted{}, nil)
		defer workload.AssertExpectations(t)

		response, err := NewUseCase(&workload, workloadtest.Translator()).Execute(context.Background(), &Request{UUID: "vm-uuid", OwnerUUID: workloadtest.OwnerUUID})
		require.NoError(t, err)

		presented, err := json.Marshal(response)
		require.NoError(t, err)

		assert.JSONEq(t, `{"dependents": [], "recreated": false}`, string(presented))
	})

	for _, answer := range workloadtest.Answers() {
		t.Run(answer.Name, func(t *testing.T) {
			t.Parallel()

			var workload controlplane.MockClient
			workload.On("VMDependents", mock.Anything, workloadtest.OwnerUUID, "vm-uuid").Once().Return(workloadControlPlane.Deleted{}, answer.Err)
			defer workload.AssertExpectations(t)

			response, err := NewUseCase(&workload, workloadtest.Translator()).Execute(context.Background(), &Request{UUID: "vm-uuid", OwnerUUID: workloadtest.OwnerUUID})

			if answer.Failure != nil {
				assert.ErrorIs(t, err, answer.Failure)
				assert.Nil(t, response)

				return
			}

			require.NoError(t, err)
			assert.Equal(t, answer.Refused, response.ValidationErrors)
			assert.Empty(t, response.Dependents)
		})
	}
}
