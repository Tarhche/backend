// Package restartVM stops a VM this node holds and boots it again in place.
package restartVM

import (
	"context"
	"errors"
	"fmt"

	"github.com/khanzadimahdi/testproject/application/workload/orchestrator/vm/internal/vmcommand"
	"github.com/khanzadimahdi/testproject/application/workload/orchestrator/vm/lock"
	"github.com/khanzadimahdi/testproject/domain"
	"github.com/khanzadimahdi/testproject/domain/workload/vm"
)

// UseCase restarts VMs. Restarting one that is down is booting it, which is
// what somebody who asked for a restart wants to end with.
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

// Execute restarts the VM, or reports why it could not. The error it returns
// is only ever a failure to report, which is worth asking again for.
func (uc *UseCase) Execute(ctx context.Context, request *Request) (*Response, error) {
	if validationErrors := uc.validator.Validate(request); len(validationErrors) > 0 {
		return &Response{ValidationErrors: validationErrors}, nil
	}

	release, err := uc.locks.Lock(ctx, request.VMUUID)
	if err != nil {
		return nil, err
	}
	defer release()

	if err := uc.restart(ctx, request.VMUUID); err != nil {
		if ctx.Err() != nil {
			return nil, err
		}

		if err := vmcommand.Failed(ctx, uc.producer, uc.nodeName, request.VMUUID, err); err != nil {
			return nil, err
		}
	}

	return &Response{}, nil
}

func (uc *UseCase) restart(ctx context.Context, vmUUID string) error {
	held, err := uc.engine.Inspect(ctx, vmUUID)
	if errors.Is(err, domain.ErrNotExists) {
		return fmt.Errorf("this node does not hold the vm: %w", err)
	}

	if err != nil {
		return err
	}

	defer uc.connections.Forget(vmUUID)

	if held.State != vm.InstanceRunning {
		return uc.engine.Start(ctx, vmUUID)
	}

	return uc.engine.Restart(ctx, vmUUID)
}
