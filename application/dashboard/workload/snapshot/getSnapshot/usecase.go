package getSnapshot

import (
	"context"

	"github.com/khanzadimahdi/testproject/application/dashboard/workload/presenter"
	workloadControlPlane "github.com/khanzadimahdi/testproject/domain/workload/controlplane"
)

// UseCase reads one snapshot.
type UseCase struct {
	workload workloadControlPlane.Client
	owners   *presenter.Directory
}

func NewUseCase(workload workloadControlPlane.Client, owners *presenter.Directory) *UseCase {
	return &UseCase{workload: workload, owners: owners}
}

func (uc *UseCase) Execute(ctx context.Context, request *Request) (*Response, error) {
	s, err := uc.workload.Snapshot(ctx, request.OwnerUUID, request.UUID)
	if err != nil {
		return nil, err
	}

	owners, err := uc.owners.Of(ctx, s.OwnerUUID)
	if err != nil {
		return nil, err
	}

	return &Response{Snapshot: presenter.NewSnapshot(s, owners)}, nil
}
