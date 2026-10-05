package getVM

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/khanzadimahdi/testproject/application/dashboard/workload/workloadtest"
	"github.com/khanzadimahdi/testproject/domain/workload/vm"
	"github.com/khanzadimahdi/testproject/infrastructure/repository/mocks/workload/controlplane"
)

func TestUseCase_Execute(t *testing.T) {
	t.Parallel()

	for _, scope := range workloadtest.Scopes {
		t.Run(scope.Name, func(t *testing.T) {
			t.Parallel()

			var workload controlplane.MockClient
			workload.On("VM", mock.Anything, scope.OwnerUUID, "vm-uuid").Once().Return(workloadtest.VM(), nil)
			defer workload.AssertExpectations(t)

			response, err := NewUseCase(&workload, workloadtest.Owners(), workloadtest.IngressDomain).Execute(
				context.Background(),
				&Request{UUID: "vm-uuid", OwnerUUID: scope.OwnerUUID},
			)
			require.NoError(t, err)

			presented, err := json.Marshal(response)
			require.NoError(t, err)

			assert.JSONEq(t, workloadtest.VMJSON, string(presented))
		})
	}

	for _, answer := range workloadtest.Failures() {
		t.Run(answer.Name, func(t *testing.T) {
			t.Parallel()

			var workload controlplane.MockClient
			workload.On("VM", mock.Anything, workloadtest.OwnerUUID, "vm-uuid").Once().Return(vm.VM{}, answer.Err)
			defer workload.AssertExpectations(t)

			response, err := NewUseCase(&workload, workloadtest.Owners(), workloadtest.IngressDomain).Execute(
				context.Background(),
				&Request{UUID: "vm-uuid", OwnerUUID: workloadtest.OwnerUUID},
			)

			assert.ErrorIs(t, err, answer.Failure)
			assert.Nil(t, response)
		})
	}
}
