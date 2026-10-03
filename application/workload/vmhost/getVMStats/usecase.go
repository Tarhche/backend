// Package getVMStats says what a VM uses right now: memory as its guest sees
// it, held against what its machine was given, and CPU, network and disk as
// the host counts them where it can. A VM that does not run uses nothing, as a
// stopped container does.
package getVMStats

import (
	"context"

	"github.com/khanzadimahdi/testproject/application/workload/vmhost"
	"github.com/khanzadimahdi/testproject/domain"
)

// UseCase says what a VM uses.
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

	stats, err := uc.engine.Stats(ctx, request.ID)
	if err != nil {
		return nil, err
	}

	return &Response{Stats: stats}, nil
}
