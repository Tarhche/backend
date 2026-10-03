// Package getVMLogs hands over what a VM's task wrote, line by line: from
// vmhost's own copy, numbered across every boot of the VM, so that a reader
// who resumes after the last number it saw misses nothing and sees nothing
// twice, however many times the VM was started meanwhile, and a VM that ended
// is read like one that runs. With follow, what comes is handed over as it
// comes, until the task has ended and nothing will write any more of it.
package getVMLogs

import (
	"context"

	"github.com/khanzadimahdi/testproject/application/workload/vmhost"
	"github.com/khanzadimahdi/testproject/domain"
	"github.com/khanzadimahdi/testproject/domain/workload/vm"
)

// UseCase hands over a VM's output.
type UseCase struct {
	engine    *vmhost.Engine
	validator domain.Validator
}

func NewUseCase(engine *vmhost.Engine, validator domain.Validator) *UseCase {
	return &UseCase{engine: engine, validator: validator}
}

// Execute hands the VM's output to emit. Once the VM is found, and before the
// first line, it calls begin, which is when whoever streams the lines can say
// that it will: a reader that follows may wait long for the first one.
func (uc *UseCase) Execute(ctx context.Context, request *Request, begin func(), emit func(vm.LogLine) error) (*Response, error) {
	if validationErrors := uc.validator.Validate(request); len(validationErrors) > 0 {
		return &Response{ValidationErrors: validationErrors}, nil
	}

	if _, err := uc.engine.VM(ctx, request.ID); err != nil {
		return nil, err
	}

	begin()

	if err := uc.engine.Logs(ctx, request.ID, request.After, request.Since, request.Follow, emit); err != nil {
		return nil, err
	}

	return &Response{}, nil
}
