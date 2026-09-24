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
	t.Run("a machine's taps are made for the user its process runs as", func(t *testing.T) {
		var (
			vmm     machineMock.MockVMM
			network machineMock.MockHostNetwork
		)

		taps := []machine.AttachedTap{{Network: "runner-isolated", Device: "rnt01234567890"}}

		vmm.On("List", mock.Anything).Once().Return([]machine.Machine{}, nil)
		vmm.On("Spawn", mock.Anything, spec()).Once().Return(machine.Machine{ID: "0123456789abcdef", Running: true, User: 1000000007}, nil)
		network.On("Plug", mock.Anything, "runner-orchestrator-01", "0123456789abcdef", 1000000007, spec().Taps).Once().Return(taps, nil)
		defer network.AssertExpectations(t)
		defer vmm.AssertExpectations(t)

		response, err := NewUseCase(&vmm, &network, accepts(), Limits{}).Execute(context.Background(), &Request{Spec: spec()})

		require.NoError(t, err)
		assert.Equal(t, machine.Machine{ID: "0123456789abcdef", Running: true, User: 1000000007, Taps: taps}, response.Machine)
	})

	t.Run("a machine that cannot be plugged in is not handed over", func(t *testing.T) {
		var (
			vmm     machineMock.MockVMM
			network machineMock.MockHostNetwork
		)

		failed := errors.New("no such network")

		vmm.On("List", mock.Anything).Once().Return([]machine.Machine{}, nil)
		vmm.On("Spawn", mock.Anything, mock.Anything).Once().Return(machine.Machine{ID: "0123456789abcdef", Running: true, User: 1000000007}, nil)
		network.On("Plug", mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything).Once().Return([]machine.AttachedTap{}, failed)
		network.On("Unplug", mock.Anything, "0123456789abcdef").Once().Return(nil)
		vmm.On("Kill", mock.Anything, "0123456789abcdef").Once().Return(nil)
		defer network.AssertExpectations(t)
		defer vmm.AssertExpectations(t)

		_, err := NewUseCase(&vmm, &network, accepts(), Limits{}).Execute(context.Background(), &Request{Spec: spec()})

		assert.ErrorIs(t, err, failed)
	})

	t.Run("a machine that does not start is not plugged in", func(t *testing.T) {
		var (
			vmm     machineMock.MockVMM
			network machineMock.MockHostNetwork
		)

		failed := errors.New("no /dev/kvm")

		vmm.On("List", mock.Anything).Once().Return([]machine.Machine{}, nil)
		vmm.On("Spawn", mock.Anything, mock.Anything).Once().Return(machine.Machine{}, failed)

		_, err := NewUseCase(&vmm, &network, accepts(), Limits{}).Execute(context.Background(), &Request{Spec: spec()})

		assert.ErrorIs(t, err, failed)
		network.AssertNotCalled(t, "Plug", mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything)
	})

	t.Run("a machine asked for again while it runs is the one running, taps and all", func(t *testing.T) {
		var (
			vmm     machineMock.MockVMM
			network machineMock.MockHostNetwork
		)

		running := machine.Machine{ID: "0123456789abcdef", Owner: "runner-orchestrator-01", Running: true, PID: 42}

		vmm.On("List", mock.Anything).Once().Return([]machine.Machine{running}, nil)

		response, err := NewUseCase(&vmm, &network, accepts(), Limits{}).Execute(context.Background(), &Request{Spec: spec()})

		require.NoError(t, err)
		assert.Equal(t, running, response.Machine)
		vmm.AssertNotCalled(t, "Spawn", mock.Anything, mock.Anything)
		network.AssertNotCalled(t, "Plug", mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything)
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
		vmm.AssertNotCalled(t, "Spawn", mock.Anything, mock.Anything)
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

	t.Run("a disk has to say where it is", func(t *testing.T) {
		asked := spec()
		asked.Files.Drives = []machine.Drive{{Path: "/var/lib/runner/images/rootfs.squashfs", ReadOnly: true}, {}}

		assert.Equal(t, "invalid_value", (&Request{Spec: asked}).Validate()["files.drives"])
	})
}
