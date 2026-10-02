// Package execVM runs a command inside a running VM, beside its task: a
// terminal, or a one-off command. It is started by the VM's agent, and its
// input, output, window size and end travel as guest frames on a connection of
// its own, which vmhost hands on byte for byte. The command outlives the
// connection, so that a terminal that is reconnected finds it; endExec ends it.
package execVM

import (
	"context"

	"github.com/khanzadimahdi/testproject/application/workload/vmhost"
	"github.com/khanzadimahdi/testproject/domain"
)

// UseCase runs commands inside VMs.
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

	execID, conn, err := uc.engine.Exec(ctx, request.ID, request.Exec)
	if err != nil {
		return nil, err
	}

	return &Response{ExecID: execID, Conn: conn}, nil
}
