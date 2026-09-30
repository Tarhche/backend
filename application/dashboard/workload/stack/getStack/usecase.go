package getStack

import (
	"context"

	"github.com/khanzadimahdi/testproject/application/dashboard/workload/presenter"
	workloadControlPlane "github.com/khanzadimahdi/testproject/domain/workload/controlplane"
)

// UseCase reads one stack and the services in it.
type UseCase struct {
	workload      workloadControlPlane.Client
	owners        *presenter.Directory
	ingressDomain string
}

func NewUseCase(workload workloadControlPlane.Client, ownerDirectory *presenter.Directory, ingressDomain string) *UseCase {
	return &UseCase{workload: workload, owners: ownerDirectory, ingressDomain: ingressDomain}
}

func (uc *UseCase) Execute(ctx context.Context, request *Request) (*Response, error) {
	s, err := uc.workload.Stack(ctx, request.UUID)
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
