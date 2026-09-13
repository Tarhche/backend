package runTask

import (
	"context"
	"errors"

	"github.com/khanzadimahdi/testproject/application/dashboard/runner/presenter"
	"github.com/khanzadimahdi/testproject/domain"
	runnerManager "github.com/khanzadimahdi/testproject/domain/runner/manager"
	"github.com/khanzadimahdi/testproject/infrastructure/runner/manager/client"
)

// UseCase hands a task to the runner. The runner is what owns a
// task's lifecycle; this decides only whether the request is well formed
// before passing it on.
type UseCase struct {
	runner        runnerManager.Client
	validator     domain.Validator
	owners        *presenter.Directory
	ingressDomain string
}

func NewUseCase(runner runnerManager.Client, validator domain.Validator, ownerDirectory *presenter.Directory, ingressDomain string) *UseCase {
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

	created, err := uc.runner.RunTask(ctx, runnerManager.TaskSpec{
		Name:    request.Name,
		Service: request.Service,
	}, request.OwnerUUID)

	// the runner validates the spec too, and it is the one that decides what it
	// can run, so what it refused is reported as it stands.
	var refused *client.ValidationError
	if errors.As(err, &refused) {
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
