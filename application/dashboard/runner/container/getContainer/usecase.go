package getContainer

import (
	"context"

	"github.com/khanzadimahdi/testproject/application/dashboard/runner/presenter"
	runnerManager "github.com/khanzadimahdi/testproject/domain/runner/manager"
)

type Response struct {
	presenter.Container
}

// UseCase reads one container.
type UseCase struct {
	runner        runnerManager.Client
	ingressDomain string
}

func NewUseCase(runner runnerManager.Client, ingressDomain string) *UseCase {
	return &UseCase{runner: runner, ingressDomain: ingressDomain}
}

func (uc *UseCase) Execute(ctx context.Context, uuid string) (*Response, error) {
	c, err := uc.runner.Container(ctx, uuid)
	if err != nil {
		return nil, err
	}

	return &Response{Container: presenter.NewContainer(c, uc.ingressDomain)}, nil
}
