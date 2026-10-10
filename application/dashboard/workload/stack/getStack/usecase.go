package getStack

import (
	"context"

	"github.com/khanzadimahdi/testproject/application/dashboard/workload/presenter"
	workloadControlPlane "github.com/khanzadimahdi/testproject/domain/workload/controlplane"
)

// UseCase reads one stack, with its compose file and the containers compose
// made for it as its VM lists them now.
type UseCase struct {
	workload workloadControlPlane.Client
	owners   *presenter.Directory
}

func NewUseCase(workload workloadControlPlane.Client, owners *presenter.Directory) *UseCase {
	return &UseCase{workload: workload, owners: owners}
}

func (uc *UseCase) Execute(ctx context.Context, request *Request) (*Response, error) {
	detail, err := uc.workload.Stack(ctx, request.OwnerUUID, request.UUID)
	if err != nil {
		return nil, err
	}

	owners, err := uc.owners.Of(ctx, detail.OwnerUUID)
	if err != nil {
		return nil, err
	}

	return &Response{StackDetail: presenter.NewStackDetail(detail, owners)}, nil
}
