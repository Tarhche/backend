// Package getVM reads one VM.
package getVM

import (
	"context"

	"github.com/khanzadimahdi/testproject/application/workload/controlplane/owner"
	"github.com/khanzadimahdi/testproject/application/workload/controlplane/presenter"
	"github.com/khanzadimahdi/testproject/domain/workload/vm"
)

type UseCase struct {
	vmRepository vm.Repository
}

func NewUseCase(vmRepository vm.Repository) *UseCase {
	return &UseCase{vmRepository: vmRepository}
}

// Execute is the VM, or domain.ErrNotExists for one that is not there, or not
// the owner's.
func (uc *UseCase) Execute(ctx context.Context, request *Request) (*Response, error) {
	v, err := owner.One(ctx, uc.vmRepository, request.OwnerUUID, request.UUID)
	if err != nil {
		return nil, err
	}

	shown := presenter.NewVM(&v)

	return &shown, nil
}
