// Package killVM ends a VM's task at once, without the grace a stop gives
// it: the task is sent KILL and returns 137, and a machine whose agent does not
// answer is ended from outside. A task killed on purpose is not started again by
// its restart policy. Killing a VM that does not run is refused with
// vm.ErrNotRunning, as it is for a container.
package killVM

import (
	"context"

	"github.com/khanzadimahdi/testproject/application/workload/vmhost"
	"github.com/khanzadimahdi/testproject/domain"
)

// UseCase kills VMs.
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

	if err := uc.engine.Kill(ctx, request.ID); err != nil {
		return nil, err
	}

	return &Response{}, nil
}
