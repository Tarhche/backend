// Package endExec ends a command run inside a VM, and everything it started,
// once nobody is attached to it any more: it is given a grace to end on its
// own, asked to stop, and then stopped. A VM that does not run took its
// commands with it, and there is nothing left to end.
package endExec

import (
	"context"

	"github.com/khanzadimahdi/testproject/application/workload/vmhost"
	"github.com/khanzadimahdi/testproject/domain"
)

// UseCase ends commands run inside VMs.
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

	ended, err := uc.engine.EndExec(ctx, request.ID, request.Exec, request.End)
	if err != nil {
		return nil, err
	}

	return &Response{Ended: ended}, nil
}
