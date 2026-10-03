// Package startVM boots a VM and runs its task in it: its machine is
// plugged into its networks, booted, told what it is, and its task started and
// looked after. A VM whose task had ended is booted again from the same disks,
// and says it is restarting until it runs. Starting a VM that runs is starting
// nothing, as it is for a container.
package startVM

import (
	"context"

	"github.com/khanzadimahdi/testproject/application/workload/vmhost"
	"github.com/khanzadimahdi/testproject/domain"
)

// UseCase boots VMs.
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

	if err := uc.engine.Start(ctx, request.ID); err != nil {
		return nil, err
	}

	return &Response{}, nil
}
