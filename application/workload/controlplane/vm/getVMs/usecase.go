// Package getVMs lists VMs, a page at a time.
package getVMs

import (
	"context"

	"github.com/khanzadimahdi/testproject/application/workload/controlplane/presenter"
	"github.com/khanzadimahdi/testproject/domain/workload/vm"
)

const (
	// Limit is how many VMs a page holds.
	Limit uint = 20

	// batch is how many VMs are read at a time when a listing has to be
	// narrowed here rather than by the store.
	batch uint = 100
)

type UseCase struct {
	vmRepository vm.Repository
}

func NewUseCase(vmRepository vm.Repository) *UseCase {
	return &UseCase{vmRepository: vmRepository}
}

func (uc *UseCase) Execute(ctx context.Context, request *Request) (*Response, error) {
	offset, page := presenter.Offset(request.Page, Limit)

	if len(request.Kind) > 0 {
		return uc.ofKind(ctx, request, offset, page)
	}

	var (
		total uint
		vms   []vm.VM
		err   error
	)

	if len(request.OwnerUUID) > 0 {
		if total, err = uc.vmRepository.CountByOwner(ctx, request.OwnerUUID); err != nil {
			return nil, err
		}

		vms, err = uc.vmRepository.GetAllByOwner(ctx, request.OwnerUUID, offset, Limit)
	} else {
		if total, err = uc.vmRepository.Count(ctx); err != nil {
			return nil, err
		}

		vms, err = uc.vmRepository.GetAll(ctx, offset, Limit)
	}

	if err != nil {
		return nil, err
	}

	return &Response{Items: presenter.NewVMs(vms), Pagination: presenter.NewPagination(total, Limit, page)}, nil
}

// ofKind is a page of the VMs of one kind. A person has a handful of VMs, so
// theirs are read whole and paged here; anybody's are read a batch at a time.
func (uc *UseCase) ofKind(ctx context.Context, request *Request, offset uint, page uint) (*Response, error) {
	var (
		vms []vm.VM
		err error
	)

	if len(request.OwnerUUID) > 0 {
		vms, err = uc.vmRepository.GetAllByOwnerAndKind(ctx, request.OwnerUUID, request.Kind)
	} else {
		vms, err = uc.everyOfKind(ctx, request.Kind)
	}

	if err != nil {
		return nil, err
	}

	total := uint(len(vms))
	if offset >= total {
		vms = nil
	} else {
		vms = vms[offset:min(offset+Limit, total)]
	}

	return &Response{Items: presenter.NewVMs(vms), Pagination: presenter.NewPagination(total, Limit, page)}, nil
}

func (uc *UseCase) everyOfKind(ctx context.Context, kind vm.Kind) ([]vm.VM, error) {
	var vms []vm.VM

	for offset := uint(0); ; offset += batch {
		read, err := uc.vmRepository.GetAll(ctx, offset, batch)
		if err != nil {
			return nil, err
		}

		for i := range read {
			if read[i].Kind == kind {
				vms = append(vms, read[i])
			}
		}

		if uint(len(read)) < batch {
			return vms, nil
		}
	}
}
