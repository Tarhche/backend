package runTask

import (
	"context"
	"errors"

	"github.com/khanzadimahdi/testproject/application/dashboard/workload/presenter"
	"github.com/khanzadimahdi/testproject/domain"
	workloadControlPlane "github.com/khanzadimahdi/testproject/domain/workload/controlplane"
	"github.com/khanzadimahdi/testproject/infrastructure/workload/controlplane/client"
)

// UseCase hands a task to the workload. The workload is what owns a
// task's lifecycle; this decides only whether the request is well formed
// before passing it on.
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

	created, err := uc.workload.RunTask(ctx, workloadControlPlane.TaskSpec{
		Name:    request.Name,
		Service: request.Service,
	}, request.OwnerUUID)

	// the workload validates the spec too, and it is the one that decides what it
	// can run, so what it refused is reported as it stands.
	if refused, ok := errors.AsType[*client.ValidationError](err); ok {
		return &Response{ValidationErrors: refused.ValidationErrors}, nil
	}

	if err != nil {
		return nil, err
	}

	people, err := uc.owners.Of(ctx, created.OwnerUUID)
	if err != nil {
		return nil, err
	}

	task := presenter.NewTask(created, uc.ingressDomain, people)

	return &Response{Task: &task}, nil
}
