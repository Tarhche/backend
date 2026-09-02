package getStack

import (
	"context"

	"github.com/khanzadimahdi/testproject/application/dashboard/runner/presenter"
	runnerManager "github.com/khanzadimahdi/testproject/domain/runner/manager"
)

type Response struct {
	presenter.Stack
}

// UseCase reads one stack and the services in it.
type UseCase struct {
	runner        runnerManager.Client
	ingressDomain string
}

func NewUseCase(runner runnerManager.Client, ingressDomain string) *UseCase {
	return &UseCase{runner: runner, ingressDomain: ingressDomain}
}

func (uc *UseCase) Execute(ctx context.Context, uuid string) (*Response, error) {
	s, err := uc.runner.Stack(ctx, uuid)
	if err != nil {
		return nil, err
	}

	return &Response{Stack: presenter.NewStack(s, uc.ingressDomain)}, nil
}
