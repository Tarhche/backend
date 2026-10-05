// Package getVMs lists VMs, a page at a time.
//
// A listing of anybody's VMs has the code runner's runs among them, newest
// first like the rest, for as long as each runs (coderunner). A run is a
// machine, so a listing of Docker VMs never has one, and it is nobody's own,
// so neither has anybody's own listing.
package getVMs

import (
	"context"

	"github.com/khanzadimahdi/testproject/application/workload/controlplane/presenter"
	"github.com/khanzadimahdi/testproject/application/workload/controlplane/vm/coderunner"
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
	runs         *coderunner.Runs
}

func NewUseCase(vmRepository vm.Repository, runs *coderunner.Runs) *UseCase {
	return &UseCase{vmRepository: vmRepository, runs: runs}
}

func (uc *UseCase) Execute(ctx context.Context, request *Request) (*Response, error) {
	offset, page := presenter.Offset(request.Page, Limit)

	runs, err := uc.runsOf(ctx, request)
	if err != nil {
		return nil, err
	}

	if len(request.Kind) > 0 {
		return uc.ofKind(ctx, request, runs, offset, page)
	}

	var (
		total uint
		vms   []vm.VM
	)

	switch {
	case len(request.OwnerUUID) > 0:
		if total, err = uc.vmRepository.CountByOwner(ctx, request.OwnerUUID); err != nil {
			return nil, err
		}

		vms, err = uc.vmRepository.GetAllByOwner(ctx, request.OwnerUUID, offset, Limit)
	case len(runs) == 0:
		if total, err = uc.vmRepository.Count(ctx); err != nil {
			return nil, err
		}

		vms, err = uc.vmRepository.GetAll(ctx, offset, Limit)
	default:
		total, vms, err = uc.withRuns(ctx, runs, offset)
	}

	if err != nil {
		return nil, err
	}

	return &Response{Items: presenter.NewVMs(vms), Pagination: presenter.NewPagination(total, Limit, page)}, nil
}

// runsOf are the runs a listing has: every one there is in a listing of
// anybody's machines, or of anybody's VMs of any kind, and none otherwise.
func (uc *UseCase) runsOf(ctx context.Context, request *Request) ([]vm.VM, error) {
	if len(request.OwnerUUID) > 0 || (len(request.Kind) > 0 && request.Kind != vm.KindMachine) {
		return nil, nil
	}

	return uc.runs.All(ctx)
}

// withRuns is the page of anybody's VMs that starts at offset, with the runs
// merged into them, and how many there are of both together.
//
// Every run is in hand, and a run only ever moves a VM further down the
// listing, never up: the VMs a page can hold are among the first offset+Limit,
// so those are read and merged with the runs, and the page cut out of that.
func (uc *UseCase) withRuns(ctx context.Context, runs []vm.VM, offset uint) (uint, []vm.VM, error) {
	total, err := uc.vmRepository.Count(ctx)
	if err != nil {
		return 0, nil, err
	}

	vms, err := uc.vmRepository.GetAll(ctx, 0, offset+Limit)
	if err != nil {
		return 0, nil, err
	}

	return total + uint(len(runs)), pageOf(coderunner.Merge(vms, runs), offset), nil
}

// ofKind is a page of the VMs of one kind. A person has a handful of VMs, so
// theirs are read whole and paged here; anybody's are read a batch at a time,
// with the runs among them when they are machines.
func (uc *UseCase) ofKind(ctx context.Context, request *Request, runs []vm.VM, offset uint, page uint) (*Response, error) {
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

	vms = coderunner.Merge(vms, runs)
	total := uint(len(vms))

	return &Response{Items: presenter.NewVMs(pageOf(vms, offset)), Pagination: presenter.NewPagination(total, Limit, page)}, nil
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

// pageOf is the page of a listing that starts at offset.
func pageOf(vms []vm.VM, offset uint) []vm.VM {
	total := uint(len(vms))
	if offset >= total {
		return nil
	}

	return vms[offset:min(offset+Limit, total)]
}
