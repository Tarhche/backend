// Package createVM creates a VM on this node and boots it, from its image or
// from a snapshot.
package createVM

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

// UseCase creates VMs on this node.
//
// It is asked again for VMs it has already made: a command is delivered again
// when its first delivery was not acknowledged in time, and the control plane
// asks again for a VM it has not seen running. A VM this node already holds is
// what was asked for, so it is booted if it is down and left alone otherwise,
// rather than made a second time or reported as a failure.
type UseCase struct {
	engine    vm.Engine
	store     snapshot.Store
	locks     *lock.Locks
	producer  domain.Producer
	validator domain.Validator
	nodeName  string
}

func NewUseCase(
	engine vm.Engine,
	store snapshot.Store,
	locks *lock.Locks,
	producer domain.Producer,
	validator domain.Validator,
	nodeName string,
) *UseCase {
	return &UseCase{
		engine:    engine,
		store:     store,
		locks:     locks,
		producer:  producer,
		validator: validator,
		nodeName:  nodeName,
	}
}

// Execute creates the VM, or reports why it could not. The error it returns is
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

	if err := uc.create(ctx, request); err != nil {
		// a node going away has not failed to create anything: whoever takes
		// the command next does.
		if ctx.Err() != nil {
			return nil, err
		}

		if err := vmcommand.Failed(ctx, uc.producer, uc.nodeName, request.VMUUID, err); err != nil {
			return nil, err
		}
	}

	return &Response{}, nil
}

func (uc *UseCase) create(ctx context.Context, request *Request) error {
	spec := vmcommand.Spec(request.VMUUID, request.Spec)

	held, err := uc.engine.Inspect(ctx, request.VMUUID)
	switch {
	case err == nil:
		if held.State != vm.InstanceRunning {
			if err := uc.engine.Start(ctx, request.VMUUID); err != nil {
				return err
			}
		}

		return uc.restored(ctx, request)
	case !errors.Is(err, domain.ErrNotExists):
		return err
	}

	if len(request.SnapshotUUID) == 0 {
		_, err := uc.engine.Create(ctx, spec)

		return err
	}

	if err := vmcommand.Restore(ctx, uc.engine, uc.store, spec, request.SnapshotUUID); err != nil {
		return err
	}

	return uc.restored(ctx, request)
}

// restored says a VM made from a snapshot has that snapshot's disk. It is said
// again when the VM is asked for again, so a first saying that was lost is not
// the last.
func (uc *UseCase) restored(ctx context.Context, request *Request) error {
	if len(request.SnapshotUUID) == 0 {
		return nil
	}

	return vmcommand.Publish(ctx, uc.producer, events.VMRestoredName, events.VMRestored{
		VMUUID:       request.VMUUID,
		NodeName:     uc.nodeName,
		SnapshotUUID: request.SnapshotUUID,
		At:           time.Now(),
	})
}
