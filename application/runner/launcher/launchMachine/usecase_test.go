package launchMachine

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/khanzadimahdi/testproject/domain"
	"github.com/khanzadimahdi/testproject/domain/runner/machine"
	machineMock "github.com/khanzadimahdi/testproject/infrastructure/repository/mocks/runner/machine"
	"github.com/khanzadimahdi/testproject/infrastructure/validator"
)

func accepts() *validator.MockValidator {
	v := &validator.MockValidator{}
	v.On("Validate", mock.Anything).Return(domain.ValidationErrors{})

	return v
}

func spec() machine.Spec {
	return machine.Spec{
		ID:        "0123456789abcdef",
		Owner:     "runner-orchestrator-01",
		VCPUs:     1,
		MemoryMiB: 256,
		Taps:      []machine.Tap{{Network: "runner-isolated"}},
		Files:     machine.Files{Kernel: "/var/lib/runner/boot/vmlinux", Initrd: "/var/lib/runner/boot/initrd.cpio.gz"},
	}
}

func TestUseCase_Execute(t *testing.T) {
	t.Run("a machine is plugged in before its process starts", func(t *testing.T) {
		var (
			vmm     machineMock.MockVMM
			network machineMock.MockHostNetwork
		)

		taps := []machine.AttachedTap{{Network: "runner-isolated", Device: "rnt01234567890"}}
		launched := machine.Machine{ID: "0123456789abcdef", Running: true}

		network.On("Plug", mock.Anything, "runner-orchestrator-01", "0123456789abcdef", spec().Taps).Once().Return(taps, nil)
		vmm.On("Spawn", mock.Anything, spec(), taps).Once().Return(launched, nil)
		defer network.AssertExpectations(t)
		defer vmm.AssertExpectations(t)

		response, err := NewUseCase(&vmm, &network, accepts(), Limits{}).Execute(context.Background(), &Request{Spec: spec()})

		require.NoError(t, err)
		assert.Equal(t, launched, response.Machine)
	})

	t.Run("a machine that does not start is unplugged again", func(t *testing.T) {
		var (
			vmm     machineMock.MockVMM
			network machineMock.MockHostNetwork
		)

		failed := errors.New("no /dev/kvm")

		network.On("Plug", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Once().Return([]machine.AttachedTap{}, nil)
		vmm.On("Spawn", mock.Anything, mock.Anything, mock.Anything).Once().Return(machine.Machine{}, failed)
		network.On("Unplug", mock.Anything, "0123456789abcdef").Once().Return(nil)
		defer network.AssertExpectations(t)

		_, err := NewUseCase(&vmm, &network, accepts(), Limits{}).Execute(context.Background(), &Request{Spec: spec()})

		assert.ErrorIs(t, err, failed)
	})

	t.Run("more than the host can spare is refused", func(t *testing.T) {
		var (
			vmm     machineMock.MockVMM
			network machineMock.MockHostNetwork
		)

		asked := spec()
		asked.VCPUs, asked.MemoryMiB = 8, 8192

		response, err := NewUseCase(&vmm, &network, accepts(), Limits{VCPUs: 4, MemoryMiB: 4096}).
			Execute(context.Background(), &Request{Spec: asked})

		require.NoError(t, err)
		assert.Equal(t, domain.ValidationErrors{"vcpus": "too_many", "memory_mib": "too_many"}, response.ValidationErrors)
		network.AssertNotCalled(t, "Plug", mock.Anything, mock.Anything, mock.Anything, mock.Anything)
	})
}

func TestRequest_Validate(t *testing.T) {
	t.Run("a machine asked for as it should be is valid", func(t *testing.T) {
		request := Request{Spec: spec()}

		assert.Empty(t, request.Validate())
	})

	t.Run("what could reach outside what the launcher names is refused", func(t *testing.T) {
		asked := spec()
		asked.ID = "../../etc"
		asked.Owner = "Somebody Else"
		asked.Taps = []machine.Tap{{Network: "docker0"}}

		validationErrors := (&Request{Spec: asked}).Validate()

		assert.Equal(t, "invalid_value", validationErrors["id"])
		assert.Equal(t, "invalid_value", validationErrors["owner"])
		assert.Equal(t, "invalid_value", validationErrors["taps"])
	})

	t.Run("a machine with nothing to boot from, or nothing to run on, is refused", func(t *testing.T) {
		validationErrors := (&Request{}).Validate()

		assert.Equal(t, domain.ValidationErrors{
			"id":           "required_field",
			"owner":        "required_field",
			"vcpus":        "invalid_value",
			"memory_mib":   "invalid_value",
			"files.kernel": "required_field",
			"files.initrd": "required_field",
		}, validationErrors)
	})

	t.Run("a machine may not use more CPU than it has", func(t *testing.T) {
		asked := spec()
		asked.CPUQuota = 1.5

		assert.Equal(t, "invalid_value", (&Request{Spec: asked}).Validate()["cpu_quota"])
	})
}
