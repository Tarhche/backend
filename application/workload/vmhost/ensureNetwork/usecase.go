// Package ensureNetwork makes a network VMs can join, if it is not there
// already: the one standalone isolated tasks share, or a stack's own, which
// its services reach each other on by name. Whether a network routes out to the
// internet is said when it is made; asking for one that is there with the other
// answer is refused with vm.ErrConflict.
package ensureNetwork

import (
	"context"

	"github.com/khanzadimahdi/testproject/application/workload/vmhost"
	"github.com/khanzadimahdi/testproject/domain"
)

// UseCase makes a network.
type UseCase struct {
	engine    *vmhost.Engine
	validator domain.Validator
}

func NewUseCase(engine *vmhost.Engine, validator domain.Validator) *UseCase {
	return &UseCase{engine: engine, validator: validator}
}

func (uc *UseCase) Execute(ctx context.Context, request *Request) (*Response, error) {
	if validationErrors := uc.validator.Validate(request); len(validationErrors) > 0 {
		return &Response{ValidationErrors: validationErrors}, nil
	}

	network, err := uc.engine.EnsureNetwork(ctx, request.Name, request.Masquerade)
	if err != nil {
		return nil, err
	}

	return &Response{Network: network}, nil
}
