// Package restartVM asks for a running VM to be stopped and booted again in place; one that is not running is brought up.
package restartVM

import (
	"context"

	"github.com/khanzadimahdi/testproject/application/workload/controlplane/owner"
	"github.com/khanzadimahdi/testproject/application/workload/controlplane/vm/lifecycle"
	"github.com/khanzadimahdi/testproject/domain"
	"github.com/khanzadimahdi/testproject/domain/workload/vm"
)

type UseCase struct {
	vmRepository vm.Repository
	lifecycle    *lifecycle.Lifecycle
	validator    domain.Validator
}

func NewUseCase(vmRepository vm.Repository, lifecycle *lifecycle.Lifecycle, validator domain.Validator) *UseCase {
	return &UseCase{vmRepository: vmRepository, lifecycle: lifecycle, validator: validator}
}

// Execute asks for it, or says why it cannot be asked. A VM that is not there,
// or not the owner's, is domain.ErrNotExists.
func (uc *UseCase) Execute(ctx context.Context, request *Request) (*Response, error) {
	if validationErrors := uc.validator.Validate(request); len(validationErrors) > 0 {
		return &Response{ValidationErrors: validationErrors}, nil
	}

	v, err := owner.One(ctx, uc.vmRepository, request.OwnerUUID, request.UUID)
	if err != nil {
		return nil, err
	}

	refused, err := lifecycle.Refused(uc.lifecycle.Restart(ctx, &v))
	if err != nil {
		return nil, err
	}

	return &Response{ValidationErrors: refused}, nil
}
