// Package reconcile holds what vmhost's records say against what the host
// holds, and puts right what differs: VMs an earlier vmhost was running are
// taken back, VMs whose machine went away meanwhile are booted again by their
// restart policy or are dead, machines nothing accounts for are ended, taps
// and addresses nothing holds are given back, the firewall is put back, the
// network VMs route out through is made, and images nothing boots are let go
// of once they take more disk than they may.
//
// vmhost does it once when it starts, before it takes any request, so that a
// request never finds a VM it has not taken back yet, and every few seconds
// after. What it finds is what vmhost says about its health until the next
// time.
package reconcile

import (
	"context"

	"github.com/khanzadimahdi/testproject/application/workload/vmhost"
)

// UseCase reconciles vmhost with its host.
type UseCase struct {
	engine *vmhost.Engine
}

func NewUseCase(engine *vmhost.Engine) *UseCase {
	return &UseCase{engine: engine}
}

func (uc *UseCase) Execute(ctx context.Context) error {
	return uc.engine.Reconcile(ctx)
}
