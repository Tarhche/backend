package runStack

import (
	"context"
	"errors"

	"github.com/khanzadimahdi/testproject/application/dashboard/runner/owners"
	"github.com/khanzadimahdi/testproject/application/dashboard/runner/presenter"
	"github.com/khanzadimahdi/testproject/domain"
	runnerManager "github.com/khanzadimahdi/testproject/domain/runner/manager"
	"github.com/khanzadimahdi/testproject/infrastructure/runner/manager/client"
)

// UseCase hands a stack to the runner.
type UseCase struct {
	runner        runnerManager.Client
	validator     domain.Validator
	owners        *owners.Directory
	ingressDomain string
}

func NewUseCase(runner runnerManager.Client, validator domain.Validator, ownerDirectory *owners.Directory, ingressDomain string) *UseCase {
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

	created, err := uc.runner.RunStack(ctx, runnerManager.StackSpec{
		Name:     request.Name,
		Services: request.Services,
	}, request.OwnerUUID)

	var refused *client.ValidationError
	if errors.As(err, &refused) {
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
