// Package createStack deploys a compose project into a Docker VM: the one the
// request names, the person's only one, or one made for it.
package createStack

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/khanzadimahdi/testproject/application/workload/controlplane/ask"
	"github.com/khanzadimahdi/testproject/application/workload/controlplane/presenter"
	"github.com/khanzadimahdi/testproject/application/workload/controlplane/slugs"
	"github.com/khanzadimahdi/testproject/application/workload/controlplane/stack/dispatch"
	"github.com/khanzadimahdi/testproject/application/workload/controlplane/vm/dockerVM"
	"github.com/khanzadimahdi/testproject/domain"
	"github.com/khanzadimahdi/testproject/domain/workload/stack"
	"github.com/khanzadimahdi/testproject/domain/workload/vm"
)

// saveAttempts is how many slugs a stack is given before giving up, for the
// rare one taken between being found free and being written.
const saveAttempts = 3

type UseCase struct {
	stackRepository stack.Repository
	chooser         *dockerVM.Chooser
	dispatcher      *dispatch.Dispatcher
	validator       domain.Validator

	now func() time.Time
}

func NewUseCase(stackRepository stack.Repository, chooser *dockerVM.Chooser, dispatcher *dispatch.Dispatcher, validator domain.Validator) *UseCase {
	return &UseCase{
		stackRepository: stackRepository,
		chooser:         chooser,
		dispatcher:      dispatcher,
		validator:       validator,
		now:             time.Now,
	}
}

// Execute stores the stack and asks for it to be deployed: at once into a
// Docker VM that is running, or once one still coming up has.
func (uc *UseCase) Execute(ctx context.Context, request *Request) (*Response, error) {
	if validationErrors := uc.validator.Validate(request); len(validationErrors) > 0 {
		return &Response{ValidationErrors: validationErrors}, nil
	}

	chosen, refused, err := uc.chooser.Choose(ctx, request.OwnerUUID, request.VM)
	if err != nil {
		return nil, err
	}

	if len(refused) > 0 {
		return &Response{ValidationErrors: refused}, nil
	}

	v := chosen.VM

	if ask.DockerRefusal(&v) != nil {
		return &Response{ValidationErrors: domain.ValidationErrors{"vm": "vm_not_running"}}, nil
	}

	s := stack.Stack{
		Name:          strings.TrimSpace(request.Name),
		OwnerUUID:     v.OwnerUUID,
		VMUUID:        v.UUID,
		Compose:       request.Compose,
		ExpectedState: stack.Running,
		State:         stack.Deploying,
		UpdatedAt:     uc.now(),
	}

	if v.CurrentState != vm.Running {
		s.Reason = dispatch.ReasonWaitingForVM
	}

	if err := uc.save(ctx, &s); err != nil {
		return nil, err
	}

	if v.CurrentState == vm.Running {
		if err := uc.dispatcher.Ask(ctx, &s, &v, stack.ActionUp, false); err != nil {
			return nil, err
		}
	}

	s.VMName = v.Name
	shown := presenter.NewStack(&s)

	return &Response{
		VM:    &presenter.ChosenVM{UUID: v.UUID, Name: v.Name, Created: chosen.Created},
		Stack: &shown,
	}, nil
}

// save gives the stack a slug no other stack holds, which is its compose
// project, and writes it down.
func (uc *UseCase) save(ctx context.Context, s *stack.Stack) error {
	for range saveAttempts {
		generated, err := slugs.Generate(ctx, s.Name, slugs.By(uc.stackRepository.GetOneBySlug))
		if err != nil {
			return err
		}

		s.Slug = generated

		_, err = uc.stackRepository.Save(ctx, s)
		if !errors.Is(err, domain.ErrAlreadyExists) {
			return err
		}
	}

	return slugs.ErrExhausted
}
