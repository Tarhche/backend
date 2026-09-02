package runContainer

import (
	"context"
	"errors"

	"github.com/khanzadimahdi/testproject/application/dashboard/runner/presenter"
	"github.com/khanzadimahdi/testproject/domain"
	runnerManager "github.com/khanzadimahdi/testproject/domain/runner/manager"
	"github.com/khanzadimahdi/testproject/infrastructure/runner/manager/client"
)

type Response struct {
	ValidationErrors domain.ValidationErrors `json:"errors,omitempty"`

	*presenter.Container
}

// UseCase hands a container to the runner. The runner is what owns a
// container's lifecycle; this decides only whether the request is well formed
// before passing it on.
type UseCase struct {
	runner        runnerManager.Client
	validator     domain.Validator
	ingressDomain string
}

func NewUseCase(runner runnerManager.Client, validator domain.Validator, ingressDomain string) *UseCase {
	return &UseCase{
		runner:        runner,
		validator:     validator,
		ingressDomain: ingressDomain,
	}
}

func (uc *UseCase) Execute(ctx context.Context, request *Request) (*Response, error) {
	if validationErrors := uc.validator.Validate(request); len(validationErrors) > 0 {
		return &Response{ValidationErrors: validationErrors}, nil
	}

	created, err := uc.runner.RunContainer(ctx, runnerManager.ContainerSpec{
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

	container := presenter.NewContainer(created, uc.ingressDomain)

	return &Response{Container: &container}, nil
}
