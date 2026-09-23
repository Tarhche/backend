package ensureNetwork

import (
	"context"

	"github.com/khanzadimahdi/testproject/domain"
	"github.com/khanzadimahdi/testproject/domain/runner/machine"
)

// UseCase makes one of an orchestrator's networks on this host, if it is not
// there already, and says what it is.
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

	ensured, err := uc.network.EnsureNetwork(ctx, request.Owner, request.Name, request.Masquerade)
	if err != nil {
		return nil, err
	}

	return &Response{Network: ensured}, nil
}
