// Package getSnapshot reads one snapshot.
package getSnapshot

import (
	"context"

	"github.com/khanzadimahdi/testproject/application/workload/controlplane/owner"
	"github.com/khanzadimahdi/testproject/application/workload/controlplane/presenter"
	"github.com/khanzadimahdi/testproject/domain/workload/snapshot"
)

type UseCase struct {
	snapshotRepository snapshot.Repository
}

func NewUseCase(snapshotRepository snapshot.Repository) *UseCase {
	return &UseCase{snapshotRepository: snapshotRepository}
}

// Execute is the snapshot, or domain.ErrNotExists for one that is not there, or
// not the owner's.
func (uc *UseCase) Execute(ctx context.Context, request *Request) (*Response, error) {
	s, err := owner.One(ctx, uc.snapshotRepository, request.OwnerUUID, request.UUID)
	if err != nil {
		return nil, err
	}

	shown := presenter.NewSnapshot(&s)

	return &shown, nil
}
