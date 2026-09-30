package getuserstack

import (
	"context"

	"github.com/khanzadimahdi/testproject/application/dashboard/workload/presenter"
	workloadControlPlane "github.com/khanzadimahdi/testproject/domain/workload/controlplane"
)

// UseCase reads one of somebody's own stacks and the services in it. One that
// is somebody else's is not found rather than refused.
type UseCase struct {
	workload      workloadControlPlane.Client
	owners        *presenter.Directory
	ingressDomain string
}

func NewUseCase(workload workloadControlPlane.Client, ownerDirectory *presenter.Directory, ingressDomain string) *UseCase {
	return &UseCase{workload: workload, owners: ownerDirectory, ingressDomain: ingressDomain}
}

func (uc *UseCase) Execute(ctx context.Context, request *Request) (*Response, error) {
	s, err := uc.workload.StackOf(ctx, request.OwnerUUID, request.UUID)
	if err != nil {
		return nil, err
	}

	people, err := uc.owners.Of(ctx, s.OwnerUUID)
	if err != nil {
		return nil, err
	}

	return &Response{Stack: presenter.NewStack(s, uc.ingressDomain, people)}, nil
}
