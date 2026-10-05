// Package startVM boots a VM this node holds.
package startVM

import (
	"context"
	"errors"
	"fmt"

	"github.com/khanzadimahdi/testproject/application/workload/orchestrator/vm/internal/vmcommand"
	"github.com/khanzadimahdi/testproject/application/workload/orchestrator/vm/lock"
	"github.com/khanzadimahdi/testproject/domain"
	"github.com/khanzadimahdi/testproject/domain/workload/vm"
)

// UseCase boots VMs. One that is running already is what was asked for.
type UseCase struct {
	engine    vm.Engine
	locks     *lock.Locks
	producer  domain.Producer
	validator domain.Validator
	nodeName  string
}

func NewUseCase(engine vm.Engine, locks *lock.Locks, producer domain.Producer, validator domain.Validator, nodeName string) *UseCase {
	return &UseCase{engine: engine, locks: locks, producer: producer, validator: validator, nodeName: nodeName}
}

// Execute boots the VM, or reports why it could not. The error it returns is
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

	if err := uc.start(ctx, request.VMUUID); err != nil {
		if ctx.Err() != nil {
			return nil, err
		}

		if err := vmcommand.Failed(ctx, uc.producer, uc.nodeName, request.VMUUID, err); err != nil {
			return nil, err
		}
	}

	return &Response{}, nil
}

func (uc *UseCase) start(ctx context.Context, vmUUID string) error {
	held, err := uc.engine.Inspect(ctx, vmUUID)
	if errors.Is(err, domain.ErrNotExists) {
		// a VM can only be booted where its disk is, and that is not here.
		return fmt.Errorf("this node does not hold the vm: %w", err)
	}

	if err != nil {
		return err
	}

	if held.State == vm.InstanceRunning {
		return nil
	}

	return uc.engine.Start(ctx, vmUUID)
}
