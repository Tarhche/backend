// Package getSnapshots lists snapshots, a page at a time.
package getSnapshots

import (
	"context"

	"github.com/khanzadimahdi/testproject/application/workload/controlplane/presenter"
	"github.com/khanzadimahdi/testproject/domain/workload/snapshot"
)

// Limit is how many snapshots a page holds.
const Limit uint = 20

type UseCase struct {
	snapshotRepository snapshot.Repository
}

func NewUseCase(snapshotRepository snapshot.Repository) *UseCase {
	return &UseCase{snapshotRepository: snapshotRepository}
}

func (uc *UseCase) Execute(ctx context.Context, request *Request) (*Response, error) {
	offset, page := presenter.Offset(request.Page, Limit)

	if len(request.VMUUID) > 0 {
		return uc.ofVM(ctx, request, offset, page)
	}

	var (
		total     uint
		snapshots []snapshot.Snapshot
		err       error
	)

	if len(request.OwnerUUID) > 0 {
		if total, err = uc.snapshotRepository.CountByOwner(ctx, request.OwnerUUID); err != nil {
			return nil, err
		}

		snapshots, err = uc.snapshotRepository.GetAllByOwner(ctx, request.OwnerUUID, offset, Limit)
	} else {
		if total, err = uc.snapshotRepository.Count(ctx); err != nil {
			return nil, err
		}

		snapshots, err = uc.snapshotRepository.GetAll(ctx, offset, Limit)
	}

	if err != nil {
		return nil, err
	}

	return &Response{Items: presenter.NewSnapshots(snapshots), Pagination: presenter.NewPagination(total, Limit, page)}, nil
}

// ofVM is a page of the snapshots taken of one VM, which are few enough to be
// read whole and paged here.
func (uc *UseCase) ofVM(ctx context.Context, request *Request, offset uint, page uint) (*Response, error) {
	taken, err := uc.snapshotRepository.GetAllByVM(ctx, request.VMUUID)
	if err != nil {
		return nil, err
	}

	snapshots := make([]snapshot.Snapshot, 0, len(taken))
	for i := range taken {
		if len(request.OwnerUUID) == 0 || taken[i].OwnerUUID == request.OwnerUUID {
			snapshots = append(snapshots, taken[i])
		}
	}

	total := uint(len(snapshots))
	if offset >= total {
		snapshots = nil
	} else {
		snapshots = snapshots[offset:min(offset+Limit, total)]
	}

	return &Response{Items: presenter.NewSnapshots(snapshots), Pagination: presenter.NewPagination(total, Limit, page)}, nil
}
