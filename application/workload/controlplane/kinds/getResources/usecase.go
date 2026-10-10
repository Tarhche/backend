// Package getResources lists the resources of any kind, a page at a time.
//
// A listing has the kind's extras among its records, newest first like the
// rest, as far as it lets them through as it lets records through: whose they
// are, what they live in and how they are labelled. The code runner's runs
// are the guest's and live in nothing, so only a listing of anybody's VMs has
// them; what a Docker VM's dockerd holds that a stack or its terminal made is
// its VM's owner's, and lives in the VM.
package getResources

import (
	"context"
	"fmt"
	"strings"

	"github.com/khanzadimahdi/testproject/application/workload/controlplane/presenter"
	"github.com/khanzadimahdi/testproject/domain/workload/kind"
	"github.com/khanzadimahdi/testproject/domain/workload/resource"
)

// Limit is how many resources a page holds.
const Limit uint = 20

type UseCase struct {
	registry  *kind.Registry[kind.ControlPlaneBinding]
	resources resource.Repository
}

func NewUseCase(registry *kind.Registry[kind.ControlPlaneBinding], resources resource.Repository) *UseCase {
	return &UseCase{registry: registry, resources: resources}
}

// Execute is the page asked for, or kind.ErrUnknownKind for a kind not run
// here.
func (uc *UseCase) Execute(ctx context.Context, request *Request) (*Response, error) {
	binding, registered := uc.registry.Lookup(request.Kind)
	if !registered {
		return nil, fmt.Errorf("%w: %q", kind.ErrUnknownKind, request.Kind)
	}

	d := binding.Descriptor()
	offset, page := presenter.Offset(request.Page, Limit)

	filter := resource.Filter{OwnerUUID: request.OwnerUUID, Labels: request.Labels}
	if len(request.Parent) > 0 {
		filter.Parent = kind.Reference{Kind: d.Parent, UUID: request.Parent}
	}

	extras, err := uc.extras(ctx, binding, filter)
	if err != nil {
		return nil, err
	}

	var (
		items []kind.Raw
		total uint
	)

	if len(extras) == 0 {
		items, total, err = uc.records(ctx, d.Name, filter, offset, Limit)
	} else {
		items, total, err = uc.withExtras(ctx, d.Name, filter, extras, offset)
	}

	if err != nil {
		return nil, err
	}

	return &Response{Items: items, Pagination: presenter.NewPagination(total, Limit, page)}, nil
}

// records is a page of the kind's records, and how many there are.
func (uc *UseCase) records(ctx context.Context, kindName string, filter resource.Filter, offset uint, limit uint) ([]kind.Raw, uint, error) {
	records, total, err := uc.resources.GetAll(ctx, kindName, filter, offset, limit)
	if err != nil {
		return nil, 0, err
	}

	items := make([]kind.Raw, len(records))
	for i := range records {
		items[i] = records[i].Raw
	}

	return items, total, nil
}

// extras are the kind's extras a listing has: every one its filter lets
// through.
func (uc *UseCase) extras(ctx context.Context, binding kind.ControlPlaneBinding, filter resource.Filter) ([]kind.Raw, error) {
	extras, extended := binding.Extras()
	if !extended {
		return nil, nil
	}

	all, err := extras.All(ctx)
	if err != nil {
		return nil, err
	}

	kept := make([]kind.Raw, 0, len(all))
	for _, r := range all {
		if filter.Passes(r.Metadata) {
			kept = append(kept, r)
		}
	}

	return kept, nil
}

// withExtras is the page of the kind's records that starts at offset, with
// its extras merged into them, and how many there are of both together.
//
// Every extra is in hand, and an extra only ever moves a record further down
// the listing, never up: the records a page can hold are among the first
// offset+Limit, so those are read and merged with the extras, and the page
// cut out of that.
func (uc *UseCase) withExtras(ctx context.Context, kindName string, filter resource.Filter, extras []kind.Raw, offset uint) ([]kind.Raw, uint, error) {
	records, total, err := uc.records(ctx, kindName, filter, 0, offset+Limit)
	if err != nil {
		return nil, 0, err
	}

	merged := Merge(records, extras)

	end := min(offset+Limit, uint(len(merged)))
	if offset >= end {
		return []kind.Raw{}, total + uint(len(extras)), nil
	}

	return merged[offset:end], total + uint(len(extras)), nil
}

// Merge is records and extras in one listing, newest first, as each of them
// is listed already: by when they were made, and by uuid for those made at
// the same moment, as a v7 uuid orders by when it was made too.
func Merge(records []kind.Raw, extras []kind.Raw) []kind.Raw {
	merged := make([]kind.Raw, 0, len(records)+len(extras))

	for len(records) > 0 && len(extras) > 0 {
		if newestFirst(extras[0], records[0]) < 0 {
			merged, extras = append(merged, extras[0]), extras[1:]
		} else {
			merged, records = append(merged, records[0]), records[1:]
		}
	}

	merged = append(merged, records...)

	return append(merged, extras...)
}

// newestFirst orders resources by when they were made, the newest first, and
// of two made at the same moment the one whose uuid sorts last first.
func newestFirst(a kind.Raw, b kind.Raw) int {
	if order := b.Metadata.CreatedAt.Compare(a.Metadata.CreatedAt); order != 0 {
		return order
	}

	return strings.Compare(b.Metadata.UUID, a.Metadata.UUID)
}
