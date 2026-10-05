// Package stopVM stops a VM this node holds, keeping its disk.
package stopVM

import (
	"context"
	"errors"

	"github.com/khanzadimahdi/testproject/application/workload/orchestrator/vm/internal/vmcommand"
	"github.com/khanzadimahdi/testproject/application/workload/orchestrator/vm/lock"
	"github.com/khanzadimahdi/testproject/domain"
	"github.com/khanzadimahdi/testproject/domain/workload/vm"
)

// UseCase stops VMs. One that is not running, or not here at all, is already
// what was asked for: what it is doing comes back in the heartbeats either
// way.
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

// Execute stops the VM, or reports why it could not. The error it returns is
// only ever a failure to report, which is worth asking again for.
func (uc *UseCase) Execute(ctx context.Context, request *Request) (*Response, error) {
	if validationErrors := uc.validator.Validate(request); len(validationErrors) > 0 {
		return &Response{ValidationErrors: validationErrors}, nil
	}

	release, err := uc.locks.Lock(ctx, request.VMUUID)
	if err != nil {
		return nil, err
	}
	defer release()

	if err := uc.stop(ctx, request.VMUUID); err != nil {
		if ctx.Err() != nil {
			return nil, err
		}

		if err := vmcommand.Failed(ctx, uc.producer, uc.nodeName, request.VMUUID, err); err != nil {
			return nil, err
		}
	}

	return &Response{}, nil
}

func (uc *UseCase) stop(ctx context.Context, vmUUID string) error {
	// once it is down, nothing opened into it before still leads anywhere.
	defer uc.connections.Forget(vmUUID)

	held, err := uc.engine.Inspect(ctx, vmUUID)
	if errors.Is(err, domain.ErrNotExists) {
		return nil
	}

	if err != nil {
		return err
	}

	if held.State != vm.InstanceRunning && held.State != vm.InstanceCreated {
		return nil
	}

	return uc.engine.Stop(ctx, vmUUID)
}
