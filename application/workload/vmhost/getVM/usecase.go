// Package getVM says what one VM is: what it was asked to be, its labels
// among it, and what it has become. The orchestrator maps it back onto a task
// execution, and reads its state, exit code, restart count and endpoints off it.
package getVM

import (
	"context"

	"github.com/khanzadimahdi/testproject/application/workload/vmhost"
	"github.com/khanzadimahdi/testproject/domain"
)

// UseCase says what one VM is.
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

	v, err := uc.engine.VM(ctx, request.ID)
	if err != nil {
		return nil, err
	}

	return &Response{VM: v}, nil
}
