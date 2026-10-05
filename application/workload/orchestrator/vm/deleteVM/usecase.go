// Package deleteVM removes a VM from this node, disk and all.
package deleteVM

import (
	"context"
	"time"

	"github.com/khanzadimahdi/testproject/application/workload/orchestrator/vm/internal/vmcommand"
	"github.com/khanzadimahdi/testproject/application/workload/orchestrator/vm/lock"
	"github.com/khanzadimahdi/testproject/domain"
	"github.com/khanzadimahdi/testproject/domain/workload/vm"
	"github.com/khanzadimahdi/testproject/domain/workload/vm/events"
)

// UseCase deletes VMs, and says so once they are gone, so their records go
// without waiting for a heartbeat to leave them out. One that was not here is
// gone already. Its snapshots are not the node's to remove: they outlive it.
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

// Execute deletes the VM and says it is gone, or reports why it could not. The
// error it returns is only ever a failure to report, which is worth asking
// again for.
func (uc *UseCase) Execute(ctx context.Context, request *Request) (*Response, error) {
	if validationErrors := uc.validator.Validate(request); len(validationErrors) > 0 {
		return &Response{ValidationErrors: validationErrors}, nil
	}

	release, err := uc.locks.Lock(ctx, request.VMUUID)
	if err != nil {
		return nil, err
	}
	defer release()

	err = uc.engine.Delete(ctx, request.VMUUID)
	uc.connections.Forget(request.VMUUID)

	if err != nil {
		if ctx.Err() != nil {
			return nil, err
		}

		if err := vmcommand.Failed(ctx, uc.producer, uc.nodeName, request.VMUUID, err); err != nil {
			return nil, err
		}

		return &Response{}, nil
	}

	if err := vmcommand.Publish(ctx, uc.producer, events.VMDeletedName, events.VMDeleted{
		VMUUID:   request.VMUUID,
		NodeName: uc.nodeName,
		At:       time.Now(),
	}); err != nil {
		return nil, err
	}

	return &Response{}, nil
}
