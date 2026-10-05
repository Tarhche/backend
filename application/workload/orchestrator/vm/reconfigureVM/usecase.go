// Package reconfigureVM gives a VM this node holds new ports, a new network or
// new resources.
package reconfigureVM

import (
	"context"

	"github.com/khanzadimahdi/testproject/application/workload/orchestrator/vm/internal/vmcommand"
	"github.com/khanzadimahdi/testproject/application/workload/orchestrator/vm/lock"
	"github.com/khanzadimahdi/testproject/domain"
	"github.com/khanzadimahdi/testproject/domain/workload/vm"
)

// UseCase reconfigures VMs. The engine restarts a VM when it cannot change it
// while it runs, which is what applying the change takes, so whatever was
// open into it is let go of either way.
type UseCase struct {
	engine      vm.Engine
	connections vmcommand.Forgetter
	locks       *lock.Locks
	producer    domain.Producer
	validator   domain.Validator
	nodeName    string
}

func NewUseCase(
	engine vm.Engine,
	connections vmcommand.Forgetter,
	locks *lock.Locks,
	producer domain.Producer,
	validator domain.Validator,
	nodeName string,
) *UseCase {
	return &UseCase{
		engine:      engine,
		connections: connections,
		locks:       locks,
		producer:    producer,
		validator:   validator,
		nodeName:    nodeName,
	}
}

// Execute reconfigures the VM, or reports why it could not. The error it
// returns is only ever a failure to report, which is worth asking again for.
func (uc *UseCase) Execute(ctx context.Context, request *Request) (*Response, error) {
	if validationErrors := uc.validator.Validate(request); len(validationErrors) > 0 {
		return &Response{ValidationErrors: validationErrors}, nil
	}

	release, err := uc.locks.Lock(ctx, request.VMUUID)
	if err != nil {
		return nil, err
	}
	defer release()

	_, err = uc.engine.Reconfigure(ctx, vmcommand.Spec(request.VMUUID, request.Spec))
	uc.connections.Forget(request.VMUUID)

	if err != nil {
		if ctx.Err() != nil {
			return nil, err
		}

		if err := vmcommand.Failed(ctx, uc.producer, uc.nodeName, request.VMUUID, err); err != nil {
			return nil, err
		}
	}

	return &Response{}, nil
}
