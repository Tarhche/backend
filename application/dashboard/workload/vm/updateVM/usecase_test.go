package updateVM

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/khanzadimahdi/testproject/application/dashboard/workload/workloadtest"
	"github.com/khanzadimahdi/testproject/domain"
	workloadControlPlane "github.com/khanzadimahdi/testproject/domain/workload/controlplane"
	"github.com/khanzadimahdi/testproject/domain/workload/port"
	"github.com/khanzadimahdi/testproject/domain/workload/vm"
	"github.com/khanzadimahdi/testproject/infrastructure/repository/mocks/workload/controlplane"
)

func useCase(workload *controlplane.MockClient) *UseCase {
	return NewUseCase(workload, workloadtest.Validator(), workloadtest.Translator(), workloadtest.Owners(), workloadtest.IngressDomain)
}

// patch is a change, read the way the dashboard sends one.
func patch(t *testing.T, body string, ownerUUID string) *Request {
	t.Helper()

	var request Request
	require.NoError(t, json.Unmarshal([]byte(body), &request))

	request.UUID = "vm-uuid"
	request.OwnerUUID = ownerUUID

	return &request
}

func TestUseCase_Execute(t *testing.T) {
	t.Parallel()

	for _, scope := range workloadtest.Scopes {
		t.Run(scope.Name, func(t *testing.T) {
			t.Parallel()

			name := "renamed"

			var workload controlplane.MockClient
			workload.On("UpdateVM", mock.Anything, scope.OwnerUUID, "vm-uuid", workloadControlPlane.VMUpdate{Name: &name}).Once().Return(workloadtest.VM(), nil)
			defer workload.AssertExpectations(t)

			response, err := useCase(&workload).Execute(context.Background(), patch(t, `{"name": "renamed"}`, scope.OwnerUUID))
			require.NoError(t, err)

			presented, err := json.Marshal(response)
			require.NoError(t, err)

			assert.JSONEq(t, workloadtest.VMJSON, string(presented))
		})
	}

	t.Run("only what is sent is changed; the rest is left as it is", func(t *testing.T) {
		t.Parallel()

		var asked workloadControlPlane.VMUpdate

		var workload controlplane.MockClient
		workload.On("UpdateVM", mock.Anything, "", "vm-uuid", mock.Anything).Once().
			Run(func(args mock.Arguments) { asked = args.Get(3).(workloadControlPlane.VMUpdate) }).
			Return(workloadtest.VM(), nil)
		defer workload.AssertExpectations(t)

		_, err := useCase(&workload).Execute(context.Background(), patch(t, `{
			"lifetime_seconds": 7200,
			"ports": [443, 80],
			"network": {"ingress": "deny", "egress": "allow"},
			"resources": {"cpus": 4, "memory": 4294967296, "disk": 42949672960}
		}`, ""))
		require.NoError(t, err)

		assert.Nil(t, asked.Name)
		require.NotNil(t, asked.Lifetime)
		assert.Equal(t, 2*time.Hour, *asked.Lifetime)
		require.NotNil(t, asked.Ports)
		assert.Equal(t, []port.Port{80, 443}, *asked.Ports)
		assert.Equal(t, &vm.Network{Ingress: vm.AccessDeny, Egress: vm.AccessAllow}, asked.Network)
		assert.Equal(t, &vm.Resources{CPUs: 4, Memory: 4 << 30, Disk: 40 << 30}, asked.Resources)
	})

	t.Run("no ports at all is a change too, and keeping none", func(t *testing.T) {
		t.Parallel()

		var asked workloadControlPlane.VMUpdate

		var workload controlplane.MockClient
		workload.On("UpdateVM", mock.Anything, "", "vm-uuid", mock.Anything).Once().
			Run(func(args mock.Arguments) { asked = args.Get(3).(workloadControlPlane.VMUpdate) }).
			Return(workloadtest.VM(), nil)
		defer workload.AssertExpectations(t)

		_, err := useCase(&workload).Execute(context.Background(), patch(t, `{"ports": [], "lifetime_seconds": 0}`, ""))
		require.NoError(t, err)

		require.NotNil(t, asked.Ports)
		assert.Equal(t, []port.Port{}, *asked.Ports)
		require.NotNil(t, asked.Lifetime, "zero keeps it until it is deleted, which is a change")
		assert.Zero(t, *asked.Lifetime)
	})

	t.Run("a change the rules refuse never reaches the workload", func(t *testing.T) {
		t.Parallel()

		var workload controlplane.MockClient

		response, err := useCase(&workload).Execute(context.Background(), patch(t, `{
			"name": "",
			"lifetime_seconds": -5,
			"ports": [80, 80],
			"network": {"ingress": "allow"},
			"resources": {"cpus": 0, "memory": 1, "disk": 1}
		}`, ""))
		require.NoError(t, err)

		assert.Equal(t, domain.ValidationErrors{
			"name":             "this field is required",
			"lifetime_seconds": "the lifetime is a number of seconds, and zero keeps it until it is deleted",
			"ports.1":          "the same port is listed more than once",
			"network.egress":   "this field is required",
			"resources.cpus":   "the provided value should be greater than zero",
		}, response.ValidationErrors)

		workload.AssertNotCalled(t, "UpdateVM", mock.Anything, mock.Anything, mock.Anything, mock.Anything)
	})

	for _, answer := range workloadtest.Answers() {
		t.Run(answer.Name, func(t *testing.T) {
			t.Parallel()

			var workload controlplane.MockClient
			workload.On("UpdateVM", mock.Anything, workloadtest.OwnerUUID, "vm-uuid", mock.Anything).Once().Return(vm.VM{}, answer.Err)
			defer workload.AssertExpectations(t)

			response, err := useCase(&workload).Execute(context.Background(), patch(t, `{"name": "renamed"}`, workloadtest.OwnerUUID))

			if answer.Failure != nil {
				assert.ErrorIs(t, err, answer.Failure)
				assert.Nil(t, response)

				return
			}

			require.NoError(t, err)
			assert.Equal(t, answer.Refused, response.ValidationErrors)
			assert.Nil(t, response.VM)
		})
	}
}
