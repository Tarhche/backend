// Package stopStack stops a stack's containers, keeping them.
package stopStack

import (
	"context"
	"errors"

	"github.com/khanzadimahdi/testproject/application/workload/controlplane/ask"
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
}

func NewUseCase(stackRepository stack.Repository, vmRepository vm.Repository, dispatcher *dispatch.Dispatcher, validator domain.Validator) *UseCase {
	return &UseCase{stackRepository: stackRepository, vmRepository: vmRepository, dispatcher: dispatcher, validator: validator}
}

// Execute asks for it, or says why it cannot be asked. A stack already where
// it was asked to go, or on its way there, is left as it is. A compose command
// is already under way on one that is on its way somewhere else, and another
// is not run beside it. A stack that is not there, or not the owner's, is
// domain.ErrNotExists.
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
		return &Response{ValidationErrors: domain.ValidationErrors{"vm": "vm_not_running"}}, nil
	} else if err != nil {
		return nil, err
	}

	var (
		next   stack.State
		action stack.Action
	)

	switch s.State {
	case stack.Stopped, stack.Stopping:
		return &Response{}, nil
	case stack.Running, stack.Failed:
		next, action = stack.Stopping, stack.ActionStop
	default:
		return &Response{ValidationErrors: domain.ValidationErrors{"stack": "invalid_state_transition"}}, nil
	}

	// only the dockerd of a VM that is up, or coming up, can run it.
	if ask.DockerRefusal(&v) != nil {
		return &Response{ValidationErrors: domain.ValidationErrors{"vm": "vm_not_running"}}, nil
	}

	if err := uc.dispatcher.Move(ctx, &s, &v, next, stack.Stopped, action); err != nil {
		return nil, err
	}

	return &Response{}, nil
}
