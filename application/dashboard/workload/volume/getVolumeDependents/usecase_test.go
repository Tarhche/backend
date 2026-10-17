package getVolumeDependents

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/khanzadimahdi/testproject/application/dashboard/workload/presenter"
	"github.com/khanzadimahdi/testproject/application/dashboard/workload/workloadtest"
	workloadControlPlane "github.com/khanzadimahdi/testproject/domain/workload/controlplane"
	"github.com/khanzadimahdi/testproject/infrastructure/repository/mocks/workload/controlplane"
)

func TestUseCase_Execute(t *testing.T) {
	t.Parallel()

	for _, scope := range workloadtest.Scopes {
		t.Run(scope.Name, func(t *testing.T) {
			t.Parallel()

			var workload controlplane.MockClient
			workload.On("VolumeDependents", mock.Anything, scope.OwnerUUID, "vm-uuid", "data").Once().Return(workloadControlPlane.Deleted{
				Dependents: []workloadControlPlane.Dependent{{Kind: "stack", UUID: "stack-uuid", Name: "shop", Recreates: true}},
				Recreated:  true,
			}, nil)
			defer workload.AssertExpectations(t)

			response, err := NewUseCase(&workload, workloadtest.Translator()).Execute(context.Background(), &Request{VMUUID: "vm-uuid", Name: "data", OwnerUUID: scope.OwnerUUID})
			require.NoError(t, err)

			assert.Empty(t, response.ValidationErrors)
			assert.Equal(t, []presenter.Dependent{{Kind: "stack", UUID: "stack-uuid", Name: "shop", Recreates: true}}, response.Dependents, "the stack that mounts it")
			assert.True(t, response.Recreated)
		})
	}

	t.Run("one nothing needs is answered with a list of none, and nothing else", func(t *testing.T) {
		t.Parallel()

		var workload controlplane.MockClient
		workload.On("VolumeDependents", mock.Anything, "", "vm-uuid", "data").Once().Return(workloadControlPlane.Deleted{}, nil)
		defer workload.AssertExpectations(t)

		response, err := NewUseCase(&workload, workloadtest.Translator()).Execute(context.Background(), &Request{VMUUID: "vm-uuid", Name: "data"})
		require.NoError(t, err)

		presented, err := json.Marshal(response)
		require.NoError(t, err)

		assert.JSONEq(t, `{"dependents": [], "recreated": false}`, string(presented))
	})

	for _, answer := range workloadtest.Answers() {
		t.Run(answer.Name, func(t *testing.T) {
			t.Parallel()

			var workload controlplane.MockClient
			workload.On("VolumeDependents", mock.Anything, workloadtest.OwnerUUID, "vm-uuid", "data").Once().Return(workloadControlPlane.Deleted{}, answer.Err)
			defer workload.AssertExpectations(t)

			response, err := NewUseCase(&workload, workloadtest.Translator()).Execute(context.Background(), &Request{VMUUID: "vm-uuid", Name: "data", OwnerUUID: workloadtest.OwnerUUID})

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
