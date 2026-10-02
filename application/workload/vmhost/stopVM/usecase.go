// Package stopVM ends a VM's task the way docker stops a container: it is
// sent TERM, given its timeout to end on its own (vm.DefaultStopTimeout unless
// asked otherwise), and then ended with everything it started; the machine
// turns itself off once it has. A task stopped on purpose is not started again
// by its restart policy. Stopping a VM that does not run is stopping nothing.
package stopVM

import (
	"context"

	"github.com/khanzadimahdi/testproject/application/workload/vmhost"
	"github.com/khanzadimahdi/testproject/domain"
)

// UseCase stops VMs.
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

	if err := uc.engine.Stop(ctx, request.ID, request.Timeout); err != nil {
		return nil, err
	}

	return &Response{}, nil
}
