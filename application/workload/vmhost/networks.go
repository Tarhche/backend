package vmhost

import (
	"context"
	"fmt"
	"slices"

	"github.com/khanzadimahdi/testproject/domain/workload/vm"
)

// EnsureNetwork makes a network VMs can join, if it is not there already, and
// says what it is: the network standalone isolated tasks share, or a stack's
// own. Whether it routes out is said when it is made.
func (e *Engine) EnsureNetwork(ctx context.Context, name string, masquerade bool) (vm.Network, error) {
	return e.fabric.EnsureNetwork(ctx, name, masquerade)
}

// RemoveNetwork takes a network away once no VM is on it: one with a VM
// running on it, or on its way back up, is refused with vm.ErrNetworkInUse,
// and asked for again by whoever removes it. The network VMs route out
// through is vmhost's own, and stays. A network that is not there is the
// outcome asked for.
func (e *Engine) RemoveNetwork(ctx context.Context, name string) error {
	if name == vm.PublicNetwork {
		return fmt.Errorf("%w: the %s network is vmhost's own", vm.ErrConflict, vm.PublicNetwork)
	}

	all, err := e.states.All(ctx)
	if err != nil {
		return err
	}

	for _, v := range all {
		if !v.State.Up() {
			continue
		}

		joined := slices.ContainsFunc(v.Spec.Networks, func(a vm.Attachment) bool { return a.Network == name })
		if joined {
			return fmt.Errorf("%w: vm %s is on %s", vm.ErrNetworkInUse, v.ID, name)
		}
	}

	return e.fabric.RemoveNetwork(ctx, name)
}
