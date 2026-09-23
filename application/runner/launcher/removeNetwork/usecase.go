package removeNetwork

import (
	"context"

	"github.com/khanzadimahdi/testproject/domain"
	"github.com/khanzadimahdi/testproject/domain/runner/machine"
)

// UseCase takes one of an orchestrator's networks away from this host. A
// network something is still plugged into is refused with
// machine.ErrNetworkInUse, for whoever asked to try again once it is free.
type UseCase struct {
	network   machine.HostNetwork
	validator domain.Validator
}

func NewUseCase(network machine.HostNetwork, validator domain.Validator) *UseCase {
	return &UseCase{
		network:   network,
		validator: validator,
	}
}

func (uc *UseCase) Execute(ctx context.Context, request *Request) (*Response, error) {
	if validationErrors := uc.validator.Validate(request); len(validationErrors) > 0 {
		return &Response{ValidationErrors: validationErrors}, nil
	}

	if err := uc.network.RemoveNetwork(ctx, request.Owner, request.Name); err != nil {
		return nil, err
	}

	return &Response{}, nil
}
