// Package deleteStack takes a stack down and removes it.
package deleteStack

import (
	"context"
	"errors"
	"time"

	"github.com/khanzadimahdi/testproject/application/workload/controlplane/owner"
	"github.com/khanzadimahdi/testproject/application/workload/controlplane/stack/dispatch"
	"github.com/khanzadimahdi/testproject/domain"
	"github.com/khanzadimahdi/testproject/domain/workload/stack"
	"github.com/khanzadimahdi/testproject/domain/workload/vm"
)

type UseCase struct {
	stackRepository stack.Repository
	vmRepository    vm.Repository
	dispatcher      *dispatch.Dispatcher
	validator       domain.Validator

	now func() time.Time
}

func NewUseCase(stackRepository stack.Repository, vmRepository vm.Repository, dispatcher *dispatch.Dispatcher, validator domain.Validator) *UseCase {
	return &UseCase{
		stackRepository: stackRepository,
		vmRepository:    vmRepository,
		dispatcher:      dispatcher,
		validator:       validator,
		now:             time.Now,
	}
}

// Execute asks for the stack to be taken down, and its record goes once it has
// been. One with nothing deployed anywhere — its VM is gone or failed, or it
// was still waiting for its VM — goes at once. One in a VM that is stopped
// waits for the VM to be started, since only its dockerd can take the project
// down. A stack that is not there, or not the owner's, is domain.ErrNotExists.
func (uc *UseCase) Execute(ctx context.Context, request *Request) (*Response, error) {
	if validationErrors := uc.validator.Validate(request); len(validationErrors) > 0 {
		return &Response{ValidationErrors: validationErrors}, nil
	}

	s, err := owner.One(ctx, uc.stackRepository, request.OwnerUUID, request.UUID)
	if err != nil {
		return nil, err
	}

	v, err := uc.vmRepository.GetOne(ctx, s.VMUUID)
	if errors.Is(err, domain.ErrNotExists) {
		return &Response{}, uc.stackRepository.Delete(ctx, s.UUID)
	} else if err != nil {
		return nil, err
	}

	switch {
	case dispatch.IsWaiting(&s), v.CurrentState == vm.Failed, v.CurrentState == vm.Deleting:
		return &Response{}, uc.stackRepository.Delete(ctx, s.UUID)

	case v.CurrentState != vm.Running && v.ExpectedState != vm.Running:
		return &Response{ValidationErrors: domain.ValidationErrors{"vm": "vm_not_running"}}, nil
	}

	s.State = stack.Removing
	s.Reason = ""
	s.UpdatedAt = uc.now()

	if _, err := uc.stackRepository.Save(ctx, &s); err != nil {
		return nil, err
	}

	if err := uc.dispatcher.Ask(ctx, &s, &v, stack.ActionDown, request.RemoveVolumes); err != nil {
		return nil, err
	}

	return &Response{Pending: true}, nil
}
