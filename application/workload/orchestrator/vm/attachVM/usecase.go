// Package attachVM opens a terminal in a VM this node holds.
package attachVM

import (
	"context"
	"fmt"

	"github.com/khanzadimahdi/testproject/domain"
	"github.com/khanzadimahdi/testproject/domain/workload/vm"
)

// shell is bash where the image has it and sh where it does not. It is asked
// of sh, which every image with a shell has, rather than tried and fallen back
// from, so a VM without bash costs nothing more than one that has it.
var shell = []string{"/bin/sh", "-c", "if [ -x /bin/bash ]; then exec /bin/bash; fi; exec /bin/sh"}

// UseCase opens terminals in VMs, for their owners alone.
//
// Whose a VM is comes off the VM itself: the control plane labelled it with
// its owner when it asked for it, so the node answers without a database and
// without taking anybody's word for it. A VM always has an owner, so one that
// does not say who is opened for nobody, and neither is anything that is not a
// VM: a task's terminal is the task's own route. Somebody who may not open it
// is told it is not there, so knowing a uuid says nothing about whether one
// exists.
type UseCase struct {
	engine    vm.Engine
	validator domain.Validator
}

func NewUseCase(engine vm.Engine, validator domain.Validator) *UseCase {
	return &UseCase{engine: engine, validator: validator}
}

func (uc *UseCase) Execute(ctx context.Context, request *Request) (vm.ExecSession, domain.ValidationErrors, error) {
	if validationErrors := uc.validator.Validate(request); len(validationErrors) > 0 {
		return nil, validationErrors, nil
	}

	held, err := uc.engine.Inspect(ctx, request.UUID)
	if err != nil {
		return nil, nil, err
	}

	owner := held.Labels[vm.LabelOwner]
	if held.Labels[vm.LabelPurpose] != vm.PurposeVM || len(owner) == 0 || owner != request.OwnerUUID {
		return nil, nil, fmt.Errorf("%w: no vm %q of theirs", domain.ErrNotExists, request.UUID)
	}

	if held.State != vm.InstanceRunning {
		return nil, domain.ValidationErrors{"uuid": "vm_is_not_running"}, nil
	}

	// the shell outlives this call: it ends when the terminal is closed, not
	// when the request that opened it is done being made.
	session, err := uc.engine.Exec(context.WithoutCancel(ctx), held.ID, vm.ExecOptions{Command: shell, TTY: true})
	if err != nil {
		return nil, nil, err
	}

	return session, nil, nil
}
