package deleteVM

import (
	"context"
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
			workload.On("DeleteVM", mock.Anything, scope.OwnerUUID, "vm-uuid", false).Once().Return(workloadControlPlane.Deleted{}, nil)
			defer workload.AssertExpectations(t)

			response, err := NewUseCase(&workload, workloadtest.Translator()).Execute(
				context.Background(),
				&Request{UUID: "vm-uuid", OwnerUUID: scope.OwnerUUID},
			)
			require.NoError(t, err)

			assert.Empty(t, response.ValidationErrors, "asked to be deleted")
			assert.Empty(t, response.Dependents, "and nothing needs it")
		})
	}

	t.Run("the containers in it are answered with, which make it again", func(t *testing.T) {
		t.Parallel()

		var workload controlplane.MockClient
		workload.On("DeleteVM", mock.Anything, workloadtest.OwnerUUID, "vm-uuid", false).Once().Return(workloadControlPlane.Deleted{
			Dependents: []workloadControlPlane.Dependent{{Kind: "container", UUID: "c-uuid", Name: "web", Recreates: true}},
			Recreated:  true,
		}, nil)
		defer workload.AssertExpectations(t)

		response, err := NewUseCase(&workload, workloadtest.Translator()).Execute(context.Background(), &Request{UUID: "vm-uuid", OwnerUUID: workloadtest.OwnerUUID})
		require.NoError(t, err)

		assert.Equal(t, []presenter.Dependent{{Kind: "container", UUID: "c-uuid", Name: "web", Recreates: true}}, response.Dependents)
		assert.True(t, response.Recreated)
	})

	t.Run("and asked with cascade, they go with it", func(t *testing.T) {
		t.Parallel()

		var workload controlplane.MockClient
		workload.On("DeleteVM", mock.Anything, workloadtest.OwnerUUID, "vm-uuid", true).Once().Return(workloadControlPlane.Deleted{
			Dependents: []workloadControlPlane.Dependent{{Kind: "container", UUID: "c-uuid", Name: "web", Recreates: true}},
		}, nil)
		defer workload.AssertExpectations(t)

		response, err := NewUseCase(&workload, workloadtest.Translator()).Execute(context.Background(), &Request{UUID: "vm-uuid", OwnerUUID: workloadtest.OwnerUUID, Cascade: true})
		require.NoError(t, err)

		assert.Len(t, response.Dependents, 1)
		assert.False(t, response.Recreated)
	})

	for _, answer := range workloadtest.Answers() {
		t.Run(answer.Name, func(t *testing.T) {
			t.Parallel()

			var workload controlplane.MockClient
			workload.On("DeleteVM", mock.Anything, workloadtest.OwnerUUID, "vm-uuid", false).Once().Return(workloadControlPlane.Deleted{}, answer.Err)
			defer workload.AssertExpectations(t)

			response, err := NewUseCase(&workload, workloadtest.Translator()).Execute(
				context.Background(),
				&Request{UUID: "vm-uuid", OwnerUUID: workloadtest.OwnerUUID},
			)

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
