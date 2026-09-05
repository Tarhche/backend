package getStack

import (
	"context"

	"github.com/khanzadimahdi/testproject/application/dashboard/runner/owners"
	"github.com/khanzadimahdi/testproject/application/dashboard/runner/presenter"
	runnerManager "github.com/khanzadimahdi/testproject/domain/runner/manager"
)

// UseCase reads one stack and the services in it.
type UseCase struct {
	runner        runnerManager.Client
	owners        *owners.Directory
	ingressDomain string
}

func NewUseCase(runner runnerManager.Client, ownerDirectory *owners.Directory, ingressDomain string) *UseCase {
	return &UseCase{runner: runner, owners: ownerDirectory, ingressDomain: ingressDomain}
}

func (uc *UseCase) Execute(ctx context.Context, request *Request) (*Response, error) {
	s, err := uc.runner.Stack(ctx, request.UUID)
	if err != nil {
		return nil, err
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
