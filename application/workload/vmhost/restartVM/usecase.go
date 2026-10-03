// Package restartVM stops a VM's task and starts it again in a machine booted
// anew from the same disks, so what the task wrote to its root survives, as it
// does a container's restart. It says it is restarting from the moment it is
// stopped until it runs again: a task on its way back up has not ended.
package restartVM

import (
	"context"

	"github.com/khanzadimahdi/testproject/application/workload/vmhost"
	"github.com/khanzadimahdi/testproject/domain"
)

// UseCase restarts VMs.
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

	if err := uc.engine.Restart(ctx, request.ID); err != nil {
		return nil, err
	}

	return &Response{}, nil
}
