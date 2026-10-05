// Package deleteVM removes a VM: its node is asked to remove it, disk and all,
// and its record goes once the node says it no longer holds it. Its stacks go
// with it; its snapshots stay, because they outlive it.
package deleteVM

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

// Execute asks for the VM to be removed. Asking again for one already on its
// way out asks its node again, which is the outcome asked for either way. A VM
// that is not there, or not the owner's, is domain.ErrNotExists.
func (uc *UseCase) Execute(ctx context.Context, request *Request) (*Response, error) {
	if validationErrors := uc.validator.Validate(request); len(validationErrors) > 0 {
		return &Response{ValidationErrors: validationErrors}, nil
	}

	v, err := owner.One(ctx, uc.vmRepository, request.OwnerUUID, request.UUID)
	if err != nil {
		return nil, err
	}

	if err := uc.lifecycle.Remove(ctx, &v); err != nil {
		return nil, err
	}

	return &Response{}, nil
}
