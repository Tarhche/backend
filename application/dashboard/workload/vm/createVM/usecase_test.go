package createVM

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/khanzadimahdi/testproject/application/dashboard/workload/input"
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

// docker is a Docker VM asked for the way the dashboard's form asks for one.
func docker() *Request {
	var request Request
	if err := json.Unmarshal([]byte(`{
		"name": "docker-1",
		"kind": "docker",
		"resources": {"cpus": 2, "memory": 2147483648, "disk": 21474836480},
		"ports": [8080, 80],
		"network": {"ingress": "allow", "egress": "allow"},
		"persistent_disk": true,
		"lifetime_seconds": 0
	}`), &request); err != nil {
		panic(err)
	}

	request.OwnerUUID = workloadtest.OwnerUUID

	return &request
}

func TestUseCase_Execute(t *testing.T) {
	t.Parallel()

	t.Run("asks for the VM as it was asked for, for whoever asked, and answers with it", func(t *testing.T) {
		t.Parallel()

		var workload controlplane.MockClient
		workload.On("CreateVM", mock.Anything, workloadtest.OwnerUUID, workloadControlPlane.VMRequest{
			Name:           "docker-1",
			Kind:           vm.KindDocker,
			Resources:      vm.Resources{CPUs: 2, Memory: 2 << 30, Disk: 20 << 30},
			Ports:          []port.Port{80, 8080},
			Network:        vm.Network{Ingress: vm.AccessAllow, Egress: vm.AccessAllow},
			PersistentDisk: true,
		}).Once().Return(workloadtest.VM(), nil)
		defer workload.AssertExpectations(t)

		response, err := useCase(&workload).Execute(context.Background(), docker())
		require.NoError(t, err)

		// the VM itself, not wrapped: what is created is what is shown.
		presented, err := json.Marshal(response)
		require.NoError(t, err)

		assert.JSONEq(t, workloadtest.VMJSON, string(presented))
	})

	t.Run("a lifetime is seconds, and a VM from a snapshot names it", func(t *testing.T) {
		t.Parallel()

		request := docker()
		request.LifetimeSeconds = 3600
		request.SnapshotUUID = "snapshot-uuid"
		request.Resources.Disk = 0

		var asked workloadControlPlane.VMRequest

		var workload controlplane.MockClient
		workload.On("CreateVM", mock.Anything, workloadtest.OwnerUUID, mock.Anything).Once().
			Run(func(args mock.Arguments) { asked = args.Get(2).(workloadControlPlane.VMRequest) }).
			Return(workloadtest.VM(), nil)
		defer workload.AssertExpectations(t)

		_, err := useCase(&workload).Execute(context.Background(), request)
		require.NoError(t, err)

		assert.Equal(t, time.Hour, asked.Lifetime)
		assert.Equal(t, "snapshot-uuid", asked.SnapshotUUID)
		assert.Zero(t, asked.Resources.Disk, "the snapshot says how big the disk has to be")
	})

	t.Run("a request the rules refuse never reaches the workload", func(t *testing.T) {
		t.Parallel()

		var workload controlplane.MockClient

		response, err := useCase(&workload).Execute(context.Background(), &Request{
			Kind:            "kubernetes",
			Image:           "ubuntu 24.04",
			Resources:       input.Resources{Memory: 1 << 30},
			Ports:           []uint{22, 70000},
			Network:         input.Network{Ingress: "open"},
			LifetimeSeconds: -1,
		})
		require.NoError(t, err)

		assert.Equal(t, domain.ValidationErrors{
			"name":             "this field is required",
			"kind":             "the kind must be either machine or docker",
			"image":            "an image is named by a reference with no spaces in it, such as nginx:1.27",
			"resources.cpus":   "the provided value should be greater than zero",
			"resources.disk":   "the provided value should be greater than zero",
			"ports.1":          "a port is a number from 1 to 65535",
			"network.ingress":  "must be either allow or deny",
			"network.egress":   "this field is required",
			"lifetime_seconds": "the lifetime is a number of seconds, and zero keeps it until it is deleted",
		}, response.ValidationErrors)

		workload.AssertNotCalled(t, "CreateVM", mock.Anything, mock.Anything, mock.Anything)
	})

	for _, answer := range workloadtest.Answers() {
		t.Run(answer.Name, func(t *testing.T) {
			t.Parallel()

			var workload controlplane.MockClient
			workload.On("CreateVM", mock.Anything, workloadtest.OwnerUUID, mock.Anything).Once().Return(vm.VM{}, answer.Err)
			defer workload.AssertExpectations(t)

			response, err := useCase(&workload).Execute(context.Background(), docker())

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

func TestRequest_Validate(t *testing.T) {
	t.Parallel()

	t.Run("what the dashboard's form sends is enough", func(t *testing.T) {
		t.Parallel()

		assert.Empty(t, docker().Validate())
	})

	t.Run("an empty request says what it needs", func(t *testing.T) {
		t.Parallel()

		assert.Equal(t, domain.ValidationErrors{
			"name":      "required_field",
			"kind":      "required_field",
			"resources": "required_field",
			"network":   "required_field",
		}, (&Request{}).Validate())
	})

	t.Run("a VM from a snapshot may leave its disk to the snapshot", func(t *testing.T) {
		t.Parallel()

		request := docker()
		request.Resources.Disk = 0
		assert.Equal(t, domain.ValidationErrors{"resources.disk": "greater_than_zero"}, request.Validate())

		request.SnapshotUUID = "snapshot-uuid"
		assert.Empty(t, request.Validate())
	})
}
