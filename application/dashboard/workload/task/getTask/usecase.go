package getTask

import (
	"context"

	"github.com/khanzadimahdi/testproject/application/dashboard/workload/presenter"
	workloadControlPlane "github.com/khanzadimahdi/testproject/domain/workload/controlplane"
)

// UseCase reads one task.

type UseCase struct {
	workload      workloadControlPlane.Client
	owners        *presenter.Directory
	ingressDomain string
}

func NewUseCase(workload workloadControlPlane.Client, ownerDirectory *presenter.Directory, ingressDomain string) *UseCase {
	return &UseCase{workload: workload, owners: ownerDirectory, ingressDomain: ingressDomain}
}

func (uc *UseCase) Execute(ctx context.Context, request *Request) (*Response, error) {
	c, err := uc.workload.Task(ctx, request.UUID)
	if err != nil {
		return nil, err
	}

	people, err := uc.owners.Of(ctx, c.OwnerUUID)
	if err != nil {
		return nil, err
	}

	return &Response{Task: presenter.NewTask(c, uc.ingressDomain, people)}, nil
}
