// Package dialVM connects to one of the ports a running VM's task is reached
// on, through its agent, which connects to the task's own address as its
// neighbours on its network would. Nothing on the host reaches into a VM's
// network and nothing is published on the host: this is how the orchestrator's
// proxy reaches a task (task.Dialer).
package dialVM

import (
	"context"

	"github.com/khanzadimahdi/testproject/application/workload/vmhost"
	"github.com/khanzadimahdi/testproject/domain"
)

// UseCase connects to VMs' tasks.
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

	conn, err := uc.engine.Dial(ctx, request.ID, request.Port)
	if err != nil {
		return nil, err
	}

	return &Response{Conn: conn}, nil
}
