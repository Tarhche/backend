package getVMs

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/khanzadimahdi/testproject/application/dashboard/workload/workloadtest"
	workloadControlPlane "github.com/khanzadimahdi/testproject/domain/workload/controlplane"
	"github.com/khanzadimahdi/testproject/domain/workload/vm"
	"github.com/khanzadimahdi/testproject/infrastructure/repository/mocks/workload/controlplane"
)

func useCase(workload *controlplane.MockClient) *UseCase {
	return NewUseCase(workload, workloadtest.Validator(), workloadtest.Owners(), workloadtest.IngressDomain)
}

func TestUseCase_Execute(t *testing.T) {
	t.Parallel()

	for _, scope := range workloadtest.Scopes {
		t.Run(scope.Name, func(t *testing.T) {
			t.Parallel()

			var workload controlplane.MockClient
			workload.On("VMs", mock.Anything, scope.OwnerUUID, uint(2)).Once().Return(workloadControlPlane.Page[vm.VM]{
				Items:       []vm.VM{workloadtest.VM()},
				TotalPages:  3,
				CurrentPage: 2,
			}, nil)
			defer workload.AssertExpectations(t)

			response, err := useCase(&workload).Execute(context.Background(), &Request{Page: 2, OwnerUUID: scope.OwnerUUID})
			require.NoError(t, err)

			presented, err := json.Marshal(response)
			require.NoError(t, err)

			assert.JSONEq(t, `{"items": [`+workloadtest.VMJSON+`], "pagination": {"total_pages": 3, "current_page": 2}}`, string(presented))
		})
	}

	t.Run("the first page, of everybody's, when the request says neither", func(t *testing.T) {
		t.Parallel()

		var workload controlplane.MockClient
		workload.On("VMs", mock.Anything, "", uint(1)).Once().Return(workloadControlPlane.Page[vm.VM]{CurrentPage: 1}, nil)
		defer workload.AssertExpectations(t)

		response, err := useCase(&workload).Execute(context.Background(), &Request{})
		require.NoError(t, err)

		presented, err := json.Marshal(response)
		require.NoError(t, err)

		assert.JSONEq(t, `{"items": [], "pagination": {"total_pages": 0, "current_page": 1}}`, string(presented))
	})

	for _, answer := range workloadtest.Failures() {
		t.Run(answer.Name, func(t *testing.T) {
			t.Parallel()

			var workload controlplane.MockClient
			workload.On("VMs", mock.Anything, workloadtest.OwnerUUID, uint(1)).Once().Return(workloadControlPlane.Page[vm.VM]{}, answer.Err)
			defer workload.AssertExpectations(t)

			response, err := useCase(&workload).Execute(context.Background(), &Request{OwnerUUID: workloadtest.OwnerUUID})

			assert.ErrorIs(t, err, answer.Failure)
			assert.Nil(t, response)
		})
	}
}
