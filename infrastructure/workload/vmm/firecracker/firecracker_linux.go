//go:build linux

package firecracker

import (
	"context"
	"log/slog"

	"github.com/khanzadimahdi/testproject/domain/workload/vm"
)

// New prepares for machines to be booted with Firecracker.
func New(config Config, logger *slog.Logger) (*Hypervisor, error) {
	return &Hypervisor{config: config, logger: logger}, nil
}

// Version is the version of the firecracker binary machines run.
func (h *Hypervisor) Version() string {
	return ""
}

func (h *Hypervisor) Boot(ctx context.Context, spec vm.MachineSpec) (vm.Machine, error) {
	return vm.Machine{}, errNotImplemented
}

func (h *Hypervisor) Machine(ctx context.Context, id string) (vm.Machine, error) {
	return vm.Machine{}, errNotImplemented
}

func (h *Hypervisor) Machines(ctx context.Context) ([]vm.Machine, error) {
	return nil, errNotImplemented
}

func (h *Hypervisor) Terminate(ctx context.Context, id string) error {
	return errNotImplemented
}
