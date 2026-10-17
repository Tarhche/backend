package deleteVolume

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/khanzadimahdi/testproject/application/dashboard/workload/presenter"
	"github.com/khanzadimahdi/testproject/application/dashboard/workload/workloadtest"
	workloadControlPlane "github.com/khanzadimahdi/testproject/domain/workload/controlplane"
	"github.com/khanzadimahdi/testproject/domain/workload/noderequest"
	"github.com/khanzadimahdi/testproject/infrastructure/repository/mocks/workload/controlplane"
)

func TestUseCase_Execute(t *testing.T) {
	t.Parallel()

	for _, scope := range workloadtest.Scopes {
		t.Run(scope.Name, func(t *testing.T) {
			t.Parallel()

			var workload controlplane.MockClient
			workload.On("DeleteVolume", mock.Anything, scope.OwnerUUID, "vm-uuid", "data", true, false).Once().Return(workloadControlPlane.Deleted{}, nil)
			defer workload.AssertExpectations(t)

			response, err := NewUseCase(&workload, workloadtest.Translator()).Execute(context.Background(), &Request{
				VMUUID:    "vm-uuid",
				Name:      "data",
				Force:     true,
				OwnerUUID: scope.OwnerUUID,
			})
			require.NoError(t, err)

			assert.Empty(t, response.ValidationErrors)
			assert.Empty(t, response.Dependents)
		})
	}

	t.Run("a refusal is answered with the containers that need it, beside it", func(t *testing.T) {
		t.Parallel()

		needs := workloadControlPlane.Deleted{Dependents: []workloadControlPlane.Dependent{{Kind: "container", UUID: "c-uuid", Name: "app", Recreates: true}}, Recreated: true}

		var workload controlplane.MockClient
		workload.On("DeleteVolume", mock.Anything, workloadtest.OwnerUUID, "vm-uuid", "data", false, false).Once().Return(needs, &noderequest.Error{Code: noderequest.CodeInvalid, Message: "remove data: volume is in use - [c-1]"})
		defer workload.AssertExpectations(t)

		response, err := NewUseCase(&workload, workloadtest.Translator()).Execute(context.Background(), &Request{VMUUID: "vm-uuid", Name: "data", OwnerUUID: workloadtest.OwnerUUID})
		require.NoError(t, err)

		assert.Contains(t, response.ValidationErrors["docker"], "volume is in use")
		assert.Equal(t, []presenter.Dependent{{Kind: "container", UUID: "c-uuid", Name: "app", Recreates: true}}, response.Dependents)
		assert.True(t, response.Recreated)
	})

	t.Run("and asked with cascade, they go first", func(t *testing.T) {
		t.Parallel()

		var workload controlplane.MockClient
		workload.On("DeleteVolume", mock.Anything, workloadtest.OwnerUUID, "vm-uuid", "data", false, true).Once().Return(workloadControlPlane.Deleted{
			Dependents: []workloadControlPlane.Dependent{{Kind: "container", UUID: "c-uuid", Name: "app", Recreates: true}},
		}, nil)
		defer workload.AssertExpectations(t)

		response, err := NewUseCase(&workload, workloadtest.Translator()).Execute(context.Background(), &Request{VMUUID: "vm-uuid", Name: "data", OwnerUUID: workloadtest.OwnerUUID, Cascade: true})
		require.NoError(t, err)

		assert.Empty(t, response.ValidationErrors)
		assert.Len(t, response.Dependents, 1)
		assert.False(t, response.Recreated)
	})

	for _, answer := range workloadtest.Answers() {
		t.Run(answer.Name, func(t *testing.T) {
			t.Parallel()

			var workload controlplane.MockClient
			workload.On("DeleteVolume", mock.Anything, workloadtest.OwnerUUID, "vm-uuid", "data", true, false).Once().Return(workloadControlPlane.Deleted{}, answer.Err)
			defer workload.AssertExpectations(t)

			response, err := NewUseCase(&workload, workloadtest.Translator()).Execute(context.Background(), &Request{
				VMUUID:    "vm-uuid",
				Name:      "data",
				Force:     true,
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
