package getStack

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
	presenter.Stack
}

// Request is one stack to show, and who is asking for it.
type Request struct {
	UUID      string `json:"-"`
	ActorUUID string `json:"-"`
}

// UseCase reads one stack and the services in it.
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
	s, err := uc.runner.Stack(ctx, request.UUID)
	if err != nil {
		return nil, err
	}

	allowed, err := uc.access.May(ctx, request.ActorUUID, permission.RunnerStacksShow, permission.SelfRunnerStacksShow, s.OwnerUUID)
	if err != nil {
		return nil, err
	}

	if !allowed {
		return nil, domain.ErrForbidden
	}

	ownerUUIDs := make([]string, 0, len(s.Services)+1)
	ownerUUIDs = append(ownerUUIDs, s.OwnerUUID)
	for i := range s.Services {
		ownerUUIDs = append(ownerUUIDs, s.Services[i].OwnerUUID)
	}

	people, err := uc.owners.Of(ctx, ownerUUIDs...)
	if err != nil {
		return nil, err
	}

	return &Response{Stack: presenter.NewStack(s, uc.ingressDomain, people)}, nil
}
