// Package getVM reads one VM, or one of the code runner's runs as the VM it
// runs in, to whoever may see anybody's.
package getVM

import (
	"context"
	"errors"

	"github.com/khanzadimahdi/testproject/application/workload/controlplane/owner"
	"github.com/khanzadimahdi/testproject/application/workload/controlplane/presenter"
	"github.com/khanzadimahdi/testproject/application/workload/controlplane/vm/coderunner"
	"github.com/khanzadimahdi/testproject/domain"
	"github.com/khanzadimahdi/testproject/domain/workload/vm"
)

type UseCase struct {
	vmRepository vm.Repository
	runs         *coderunner.Runs
}

func NewUseCase(vmRepository vm.Repository, runs *coderunner.Runs) *UseCase {
	return &UseCase{vmRepository: vmRepository, runs: runs}
}

// Execute is the VM, or domain.ErrNotExists for one that is not there, or not
// the owner's. A uuid that names no VM may name a run, which is anybody's to
// read and nobody's own.
func (uc *UseCase) Execute(ctx context.Context, request *Request) (*Response, error) {
	v, err := owner.One(ctx, uc.vmRepository, request.OwnerUUID, request.UUID)
	if errors.Is(err, domain.ErrNotExists) {
		run, runErr := uc.runs.One(ctx, request.OwnerUUID, request.UUID)
		if errors.Is(runErr, domain.ErrNotExists) {
			return nil, err
		} else if runErr != nil {
			return nil, runErr
		}

		v = coderunner.VM(&run)
	} else if err != nil {
		return nil, err
	}

	shown := presenter.NewVM(&v)

	return &shown, nil
}
