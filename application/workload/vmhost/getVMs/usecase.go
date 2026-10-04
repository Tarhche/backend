// Package getVMs lists the VMs carrying every label filter given: a node's
// own (node.name), a task's (task.uuid), the ones answering to a slug
// (task.slug). It is how an orchestrator says what it is holding without
// keeping any record of its own, several times a second, so it is answered
// from memory.
package getVMs

import (
	"context"

	"github.com/khanzadimahdi/testproject/application/workload/vmhost"
	"github.com/khanzadimahdi/testproject/domain"
)

// UseCase lists VMs.
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

	vms, err := uc.engine.VMs(ctx, request.Labels)
	if err != nil {
		return nil, err
	}

	return &Response{VMs: vms}, nil
}
