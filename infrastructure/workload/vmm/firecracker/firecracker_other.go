//go:build !linux

package firecracker

import (
	"context"
	"errors"
	"fmt"
	"log/slog"

	"github.com/khanzadimahdi/testproject/domain/workload/vm"
)

// errUnsupported is every answer off Linux: a microVM needs KVM under it.
var errUnsupported = fmt.Errorf("%w: microVMs only boot on linux, with kvm", errors.ErrUnsupported)

// New refuses: machines only boot on Linux.
func New(config Config, logger *slog.Logger) (*Hypervisor, error) {
	return nil, errUnsupported
}

func (h *Hypervisor) Version() string {
	return ""
}

func (h *Hypervisor) Boot(ctx context.Context, spec vm.MachineSpec) (vm.Machine, error) {
	return vm.Machine{}, errUnsupported
}

func (h *Hypervisor) Machine(ctx context.Context, id string) (vm.Machine, error) {
	return vm.Machine{}, errUnsupported
}

func (h *Hypervisor) Machines(ctx context.Context) ([]vm.Machine, error) {
	return nil, errUnsupported
}

func (h *Hypervisor) Terminate(ctx context.Context, id string) error {
	return errUnsupported
}
