// Package restoreVM replaces the disk of a VM this node holds with a
// snapshot's.
package restoreVM

import (
	"context"
	"errors"
	"time"

	"github.com/khanzadimahdi/testproject/application/workload/orchestrator/vm/internal/vmcommand"
	"github.com/khanzadimahdi/testproject/application/workload/orchestrator/vm/lock"
	"github.com/khanzadimahdi/testproject/domain"
	"github.com/khanzadimahdi/testproject/domain/workload/snapshot"
	"github.com/khanzadimahdi/testproject/domain/workload/vm"
	"github.com/khanzadimahdi/testproject/domain/workload/vm/events"
)

// UseCase restores VMs from snapshots.
//
// The VM is stopped, the snapshot's archive is streamed from where snapshots
// are kept into the engine, and the engine replaces the VM under the same name
// with the disk the archive holds. The VM keeps its uuid, its slug and its
// ports, and once it is back that is said, so the restore it was waiting on is
// over.
type UseCase struct {
	engine      vm.Engine
	store       snapshot.Store
	connections vmcommand.Forgetter
	locks       *lock.Locks
	producer    domain.Producer
	validator   domain.Validator
	nodeName    string
}

func NewUseCase(
	engine vm.Engine,
	store snapshot.Store,
	connections vmcommand.Forgetter,
	locks *lock.Locks,
	producer domain.Producer,
	validator domain.Validator,
	nodeName string,
) *UseCase {
	return &UseCase{
		engine:      engine,
		store:       store,
		connections: connections,
		locks:       locks,
		producer:    producer,
		validator:   validator,
		nodeName:    nodeName,
	}
}

// Execute restores the VM and says so, or reports why it could not. The error
// it returns is only ever a failure to report, which is worth asking again
// for.
func (uc *UseCase) Execute(ctx context.Context, request *Request) (*Response, error) {
	if validationErrors := uc.validator.Validate(request); len(validationErrors) > 0 {
		return &Response{ValidationErrors: validationErrors}, nil
	}

	release, err := uc.locks.Lock(ctx, request.VMUUID)
	if err != nil {
		return nil, err
	}
	defer release()

	if err := uc.restore(ctx, request); err != nil {
		if ctx.Err() != nil {
			return nil, err
		}

		if err := vmcommand.Failed(ctx, uc.producer, uc.nodeName, request.VMUUID, err); err != nil {
			return nil, err
		}

		return &Response{}, nil
	}

	if err := vmcommand.Publish(ctx, uc.producer, events.VMRestoredName, events.VMRestored{
		VMUUID:       request.VMUUID,
		NodeName:     uc.nodeName,
		SnapshotUUID: request.SnapshotUUID,
		At:           time.Now(),
	}); err != nil {
		return nil, err
	}

	return &Response{}, nil
}

func (uc *UseCase) restore(ctx context.Context, request *Request) error {
	// whatever was open into the disk being replaced leads nowhere now.
	defer uc.connections.Forget(request.VMUUID)

	// a VM that is not here is restored all the same: the engine makes it.
	if err := uc.engine.Stop(ctx, request.VMUUID); err != nil && !errors.Is(err, domain.ErrNotExists) {
		return err
	}

	spec := vmcommand.Spec(request.VMUUID, request.Spec)

	return vmcommand.Restore(ctx, uc.engine, uc.store, spec, request.SnapshotUUID)
}
