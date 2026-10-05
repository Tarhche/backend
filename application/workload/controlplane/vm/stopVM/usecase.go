// Package stopVM asks for a VM to be stopped, keeping its disk. One of the code
// runner's runs is stopped as its task is, and is taken away once it has.
package stopVM

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

// Execute asks for it, or says why it cannot be asked. A VM that is not there,
// or not the owner's, is domain.ErrNotExists.
func (uc *UseCase) Execute(ctx context.Context, request *Request) (*Response, error) {
	if validationErrors := uc.validator.Validate(request); len(validationErrors) > 0 {
		return &Response{ValidationErrors: validationErrors}, nil
	}

	v, err := owner.One(ctx, uc.vmRepository, request.OwnerUUID, request.UUID)
	if errors.Is(err, domain.ErrNotExists) {
		return uc.stopRun(ctx, request, err)
	} else if err != nil {
		return nil, err
	}

	refused, err := lifecycle.Refused(uc.lifecycle.Down(ctx, &v))
	if err != nil {
		return nil, err
	}

	return &Response{ValidationErrors: refused}, nil
}

// stopRun stops the run a uuid that names no VM may name. notThere is what
// looking for a VM came to, which is the answer when it names no run either.
func (uc *UseCase) stopRun(ctx context.Context, request *Request, notThere error) (*Response, error) {
	run, err := uc.runs.One(ctx, request.OwnerUUID, request.UUID)
	if errors.Is(err, domain.ErrNotExists) {
		return nil, notThere
	} else if err != nil {
		return nil, err
	}

	refused, err := uc.runs.Stop(ctx, &run)
	if err != nil {
		return nil, err
	}

	return &Response{ValidationErrors: refused}, nil
}
