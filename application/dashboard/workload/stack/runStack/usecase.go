package runStack

import (
	"context"
	"errors"

	"github.com/khanzadimahdi/testproject/application/dashboard/workload/presenter"
	"github.com/khanzadimahdi/testproject/domain"
	workloadControlPlane "github.com/khanzadimahdi/testproject/domain/workload/controlplane"
	"github.com/khanzadimahdi/testproject/infrastructure/workload/controlplane/client"
)

// UseCase hands a stack to the workload.
type UseCase struct {
	workload      workloadControlPlane.Client
	validator     domain.Validator
	owners        *presenter.Directory
	ingressDomain string
}

func NewUseCase(workload workloadControlPlane.Client, validator domain.Validator, ownerDirectory *presenter.Directory, ingressDomain string) *UseCase {
	return &UseCase{
		workload:      workload,
		validator:     validator,
		owners:        ownerDirectory,
		ingressDomain: ingressDomain,
	}
}

func (uc *UseCase) Execute(ctx context.Context, request *Request) (*Response, error) {
	if validationErrors := uc.validator.Validate(request); len(validationErrors) > 0 {
		return &Response{ValidationErrors: validationErrors}, nil
	}

	created, err := uc.workload.RunStack(ctx, workloadControlPlane.StackSpec{
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
