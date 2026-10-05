// Package startVM asks for a VM to be running: its node starts the instance it holds, or makes one, and a VM on no node is placed first.
package startVM

import (
	"context"

	"github.com/khanzadimahdi/testproject/application/workload/controlplane/owner"
	"github.com/khanzadimahdi/testproject/application/workload/controlplane/vm/coderunner"
	"github.com/khanzadimahdi/testproject/application/workload/controlplane/vm/lifecycle"
	"github.com/khanzadimahdi/testproject/domain"
	"github.com/khanzadimahdi/testproject/domain/workload/vm"
)

type UseCase struct {
	vmRepository vm.Repository
	runs         *coderunner.Runs
	lifecycle    *lifecycle.Lifecycle
	validator    domain.Validator
}

func NewUseCase(vmRepository vm.Repository, runs *coderunner.Runs, lifecycle *lifecycle.Lifecycle, validator domain.Validator) *UseCase {
	return &UseCase{vmRepository: vmRepository, runs: runs, lifecycle: lifecycle, validator: validator}
}

// Execute asks for it, or says why it cannot be asked. A VM that is not there,
// or not the owner's, is domain.ErrNotExists.
func (uc *UseCase) Execute(ctx context.Context, request *Request) (*Response, error) {
	if validationErrors := uc.validator.Validate(request); len(validationErrors) > 0 {
		return &Response{ValidationErrors: validationErrors}, nil
	}

	v, err := owner.One(ctx, uc.vmRepository, request.OwnerUUID, request.UUID)
	if err != nil {
		// a uuid that names no VM may name a run, which is the code runner's
		// to start.
		refused, err := uc.runs.Refused(ctx, request.OwnerUUID, request.UUID, err)
		if err != nil {
			return nil, err
		}

		return &Response{ValidationErrors: refused}, nil
	}

	refused, err := lifecycle.Refused(uc.lifecycle.Up(ctx, &v))
	if err != nil {
		return nil, err
	}

	return &Response{ValidationErrors: refused}, nil
}
