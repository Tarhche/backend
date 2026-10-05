// Package getStacks lists stacks, a page at a time.
package getStacks

import (
	"context"
	"errors"

	"github.com/khanzadimahdi/testproject/application/workload/controlplane/presenter"
	"github.com/khanzadimahdi/testproject/domain"
	"github.com/khanzadimahdi/testproject/domain/workload/stack"
	"github.com/khanzadimahdi/testproject/domain/workload/vm"
)

// Limit is how many stacks a page holds.
const Limit uint = 20

type UseCase struct {
	stackRepository stack.Repository
	vmRepository    vm.Repository
}

func NewUseCase(stackRepository stack.Repository, vmRepository vm.Repository) *UseCase {
	return &UseCase{stackRepository: stackRepository, vmRepository: vmRepository}
}

func (uc *UseCase) Execute(ctx context.Context, request *Request) (*Response, error) {
	offset, page := presenter.Offset(request.Page, Limit)

	if len(request.VMUUID) > 0 {
		return uc.inVM(ctx, request, offset, page)
	}

	var (
		total  uint
		stacks []stack.Stack
		err    error
	)

	if len(request.OwnerUUID) > 0 {
		if total, err = uc.stackRepository.CountByOwner(ctx, request.OwnerUUID); err != nil {
			return nil, err
		}

		stacks, err = uc.stackRepository.GetAllByOwner(ctx, request.OwnerUUID, offset, Limit)
	} else {
		if total, err = uc.stackRepository.Count(ctx); err != nil {
			return nil, err
		}

		stacks, err = uc.stackRepository.GetAll(ctx, offset, Limit)
	}

	if err != nil {
		return nil, err
	}

	if err := uc.named(ctx, stacks); err != nil {
		return nil, err
	}

	return &Response{Items: presenter.NewStacks(stacks), Pagination: presenter.NewPagination(total, Limit, page)}, nil
}

// inVM is a page of the stacks deployed into one VM, which are few enough to
// be read whole and paged here.
func (uc *UseCase) inVM(ctx context.Context, request *Request, offset uint, page uint) (*Response, error) {
	deployed, err := uc.stackRepository.GetAllByVM(ctx, request.VMUUID)
	if err != nil {
		return nil, err
	}

	stacks := make([]stack.Stack, 0, len(deployed))
	for i := range deployed {
		if len(request.OwnerUUID) == 0 || deployed[i].OwnerUUID == request.OwnerUUID {
			stacks = append(stacks, deployed[i])
		}
	}

	total := uint(len(stacks))
	if offset >= total {
		stacks = nil
	} else {
		stacks = stacks[offset:min(offset+Limit, total)]
	}

	if err := uc.named(ctx, stacks); err != nil {
		return nil, err
	}

	return &Response{Items: presenter.NewStacks(stacks), Pagination: presenter.NewPagination(total, Limit, page)}, nil
}

// named gives each stack what its VM is called now. The name is read rather
// than kept, so a VM that is renamed is named anew; each VM of the page is read
// once, however many of its stacks there are, and one that is gone leaves its
// stacks unnamed.
func (uc *UseCase) named(ctx context.Context, stacks []stack.Stack) error {
	names := make(map[string]string)

	for i := range stacks {
		name, read := names[stacks[i].VMUUID]
		if !read {
			v, err := uc.vmRepository.GetOne(ctx, stacks[i].VMUUID)

			switch {
			case errors.Is(err, domain.ErrNotExists):
			case err != nil:
				return err
			default:
				name = v.Name
			}

			names[stacks[i].VMUUID] = name
		}

		stacks[i].VMName = name
	}

	return nil
}
