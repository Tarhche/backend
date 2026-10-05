// Package deleteVM removes a VM: its node is asked to remove it, disk and all,
// and its record goes once the node says it no longer holds it. Its stacks go
// with it; its snapshots stay, because they outlive it.
//
// One of the code runner's runs is taken away as its task is, whether or not it
// is still running.
package deleteVM

import (
	"context"
	"errors"

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

// Execute asks for the VM to be removed. Asking again for one already on its
// way out asks its node again, which is the outcome asked for either way. A VM
// that is not there, or not the owner's, is domain.ErrNotExists.
func (uc *UseCase) Execute(ctx context.Context, request *Request) (*Response, error) {
	if validationErrors := uc.validator.Validate(request); len(validationErrors) > 0 {
		return &Response{ValidationErrors: validationErrors}, nil
	}

	v, err := owner.One(ctx, uc.vmRepository, request.OwnerUUID, request.UUID)
	if errors.Is(err, domain.ErrNotExists) {
		return uc.deleteRun(ctx, request, err)
	} else if err != nil {
		return nil, err
	}

	if err := uc.lifecycle.Remove(ctx, &v); err != nil {
		return nil, err
	}

	return &Response{}, nil
}

// deleteRun takes away the run a uuid that names no VM may name. notThere is
// what looking for a VM came to, which is the answer when it names no run
// either.
func (uc *UseCase) deleteRun(ctx context.Context, request *Request, notThere error) (*Response, error) {
	run, err := uc.runs.One(ctx, request.OwnerUUID, request.UUID)
	if errors.Is(err, domain.ErrNotExists) {
		return nil, notThere
	} else if err != nil {
		return nil, err
	}

	if err := uc.runs.Delete(ctx, &run); err != nil {
		return nil, err
	}

	return &Response{}, nil
}
