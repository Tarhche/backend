package getContainer

import (
	"context"

	"github.com/khanzadimahdi/testproject/application/access"
	"github.com/khanzadimahdi/testproject/application/dashboard/runner/owners"
	"github.com/khanzadimahdi/testproject/application/dashboard/runner/presenter"
	"github.com/khanzadimahdi/testproject/domain"
	"github.com/khanzadimahdi/testproject/domain/permission"
	runnerManager "github.com/khanzadimahdi/testproject/domain/runner/manager"
)

type Response struct {
	presenter.Container
}

// UseCase reads one container.
// Request is one container to show, and who is asking for it.
type Request struct {
	UUID      string `json:"-"`
	ActorUUID string `json:"-"`
}

type UseCase struct {
	runner        runnerManager.Client
	owners        *owners.Directory
	access        *access.Guard
	ingressDomain string
}

func NewUseCase(runner runnerManager.Client, ownerDirectory *owners.Directory, guard *access.Guard, ingressDomain string) *UseCase {
	return &UseCase{runner: runner, owners: ownerDirectory, access: guard, ingressDomain: ingressDomain}
}

func (uc *UseCase) Execute(ctx context.Context, request *Request) (*Response, error) {
	c, err := uc.runner.Container(ctx, request.UUID)
	if err != nil {
		return nil, err
	}

	allowed, err := uc.access.May(ctx, request.ActorUUID, permission.RunnerContainersShow, permission.SelfRunnerContainersShow, c.OwnerUUID)
	if err != nil {
		return nil, err
	}

	if !allowed {
		return nil, domain.ErrForbidden
	}

	people, err := uc.owners.Of(ctx, c.OwnerUUID)
	if err != nil {
		return nil, err
	}

	return &Response{Container: presenter.NewContainer(c, uc.ingressDomain, people)}, nil
}
