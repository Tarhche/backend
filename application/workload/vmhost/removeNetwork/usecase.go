// Package removeNetwork takes a network away once no VM is on it. One a VM
// runs on, or is on its way back up on, is refused with vm.ErrNetworkInUse,
// which whoever removes it asks again after, as it does docker. The network
// VMs route out through is vmhost's own and stays. One that is not there is
// the outcome asked for.
package removeNetwork

import (
	"context"

	"github.com/khanzadimahdi/testproject/application/workload/vmhost"
	"github.com/khanzadimahdi/testproject/domain"
)

// UseCase takes a network away.
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

	if err := uc.engine.RemoveNetwork(ctx, request.Name); err != nil {
		return nil, err
	}

	return &Response{}, nil
}
