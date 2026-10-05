// Package getVMLogs reads the log of a VM this node holds, which nothing
// stores: it is asked of the engine whenever somebody wants to read it.
package getVMLogs

import (
	"context"

	"github.com/khanzadimahdi/testproject/domain"
	"github.com/khanzadimahdi/testproject/domain/workload/noderequest"
	"github.com/khanzadimahdi/testproject/domain/workload/vm"
)

// UseCase reads VMs' logs. A reply has to fit in one NATS message, so a log is
// read from its end and cut at the most lines a reply carries; one cut short
// says so.
type UseCase struct {
	engine    vm.Engine
	validator domain.Validator
}

func NewUseCase(engine vm.Engine, validator domain.Validator) *UseCase {
	return &UseCase{engine: engine, validator: validator}
}

func (uc *UseCase) Execute(ctx context.Context, request *Request) (*Response, error) {
	if validationErrors := uc.validator.Validate(request); len(validationErrors) > 0 {
		return &Response{ValidationErrors: validationErrors}, nil
	}

	// one line more than a reply carries is how a log that does not fit is
	// told from one that just does.
	tail := request.Tail
	if tail == 0 || tail > noderequest.MaxLogLines {
		tail = noderequest.MaxLogLines + 1
	}

	lines, err := uc.engine.Logs(ctx, request.VMUUID, vm.LogOptions{Since: request.Since, Tail: tail})
	if err != nil {
		return nil, err
	}

	response := &Response{Lines: lines}

	if len(lines) > noderequest.MaxLogLines {
		response.Lines = lines[len(lines)-noderequest.MaxLogLines:]
		response.Truncated = true
	}

	return response, nil
}
