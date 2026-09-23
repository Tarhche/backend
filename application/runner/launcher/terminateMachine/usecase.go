package terminateMachine

import (
	"context"

	"github.com/khanzadimahdi/testproject/domain"
	"github.com/khanzadimahdi/testproject/domain/runner/machine"
)

// UseCase ends a machine's process and unplugs it. A machine that is already
// gone is the outcome asked for, so this can be asked for as often as it takes.
type UseCase struct {
	vmm       machine.VMM
	network   machine.HostNetwork
	validator domain.Validator
}

func NewUseCase(vmm machine.VMM, network machine.HostNetwork, validator domain.Validator) *UseCase {
	return &UseCase{
		vmm:       vmm,
		network:   network,
		validator: validator,
	}
}

func (uc *UseCase) Execute(ctx context.Context, request *Request) (*Response, error) {
	if validationErrors := uc.validator.Validate(request); len(validationErrors) > 0 {
		return &Response{ValidationErrors: validationErrors}, nil
	}

	// the process goes first: a machine unplugged while it runs is a machine
	// whose network device vanished under it.
	if err := uc.vmm.Kill(ctx, request.ID); err != nil {
		return nil, err
	}

	if err := uc.network.Unplug(ctx, request.ID); err != nil {
		return nil, err
	}

	return &Response{}, nil
}
