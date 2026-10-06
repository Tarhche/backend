// Package parents says what VMs are doing to what lives in them: whether the
// stacks in a Docker VM can be looked at, and why not when they cannot.
//
// It is how the control plane's observer knows what a stack in a VM its node
// did not look into is waiting on, until VMs are a kind of their own and say
// so themselves.
package parents

import (
	"context"
	"errors"

	"github.com/khanzadimahdi/testproject/application/workload/controlplane/kinds/observe"
	"github.com/khanzadimahdi/testproject/application/workload/controlplane/vm/lifecycle"
	"github.com/khanzadimahdi/testproject/domain"
	"github.com/khanzadimahdi/testproject/domain/workload/kind"
	"github.com/khanzadimahdi/testproject/domain/workload/vm"
)

// VMs are the VMs the control plane keeps, as the parents of what lives in
// them.
type VMs struct {
	vms vm.Repository
}

var _ observe.Parents = &VMs{}

func New(vms vm.Repository) *VMs {
	return &VMs{vms: vms}
}

// Down is the state a VM is in when it is not running, which is all that
// keeps what lives in it from being looked at. A VM that runs, one that is
// gone, and a parent that is not a VM, say nothing.
func (p *VMs) Down(ctx context.Context, parent kind.Reference) (kind.State, error) {
	if parent.Kind != lifecycle.ParentKind {
		return "", nil
	}

	v, err := p.vms.GetOne(ctx, parent.UUID)
	switch {
	case errors.Is(err, domain.ErrNotExists):
		return "", nil
	case err != nil:
		return "", err
	case v.CurrentState == vm.Running:
		return "", nil
	}

	return kind.State(v.CurrentState.String()), nil
}
