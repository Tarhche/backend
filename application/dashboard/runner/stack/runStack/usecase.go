package runStack

import (
	"context"
	"errors"

	"github.com/khanzadimahdi/testproject/application/dashboard/runner/presenter"
	"github.com/khanzadimahdi/testproject/domain"
	runnerControlPlane "github.com/khanzadimahdi/testproject/domain/runner/controlplane"
	"github.com/khanzadimahdi/testproject/infrastructure/runner/controlplane/client"
)

// UseCase hands a stack to the runner.
type UseCase struct {
	runner        runnerControlPlane.Client
	validator     domain.Validator
	owners        *presenter.Directory
	ingressDomain string
}

func NewUseCase(runner runnerControlPlane.Client, validator domain.Validator, ownerDirectory *presenter.Directory, ingressDomain string) *UseCase {
	return &UseCase{
		runner:        runner,
		validator:     validator,
		owners:        ownerDirectory,
		ingressDomain: ingressDomain,
	}
}

func (uc *UseCase) Execute(ctx context.Context, request *Request) (*Response, error) {
	if validationErrors := uc.validator.Validate(request); len(validationErrors) > 0 {
		return &Response{ValidationErrors: validationErrors}, nil
	}

	created, err := uc.runner.RunStack(ctx, runnerControlPlane.StackSpec{
		Name:     request.Name,
		Services: request.Services,
	}, request.OwnerUUID)

	if refused, ok := errors.AsType[*client.ValidationError](err); ok {
		return &Response{ValidationErrors: refused.ValidationErrors}, nil
	}

	if err != nil {
		return nil, err
	}

	ownerUUIDs := make([]string, 0, len(created.Services)+1)
	ownerUUIDs = append(ownerUUIDs, created.OwnerUUID)
	for i := range created.Services {
		ownerUUIDs = append(ownerUUIDs, created.Services[i].OwnerUUID)
	}

	people, err := uc.owners.Of(ctx, ownerUUIDs...)
	if err != nil {
		return nil, err
	}

	stack := presenter.NewStack(created, uc.ingressDomain, people)

	return &Response{Stack: &stack}, nil
}
