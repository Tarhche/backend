// Package vms keeps VMs in memory, the way the MongoDB repository keeps them:
// newest first, one slug per VM, and a copy read before a newer request is
// refused rather than written over it.
package vms

import (
	"context"
	"errors"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/gofrs/uuid/v5"

	"github.com/khanzadimahdi/testproject/domain"
	"github.com/khanzadimahdi/testproject/domain/workload/vm"
)

// ErrStale is a copy of a VM written back after the stored one was asked for
// something it does not know of.
var ErrStale = errors.New("the vm was changed after this copy of it was read")

type Repository struct {
	lock sync.Mutex
	vms  map[string]vm.VM

	// Fail, when set, is what every call reports instead of doing anything.
	Fail error
}

var _ vm.Repository = &Repository{}

func NewRepository(vms ...vm.VM) *Repository {
	r := &Repository{vms: make(map[string]vm.VM, len(vms))}

	for _, v := range vms {
		r.vms[v.UUID] = clone(v)
	}

	return r
}

func (r *Repository) GetAll(_ context.Context, offset uint, limit uint) ([]vm.VM, error) {
	return r.page(func(vm.VM) bool { return true }, offset, limit)
}

func (r *Repository) GetAllByOwner(_ context.Context, ownerUUID string, offset uint, limit uint) ([]vm.VM, error) {
	return r.page(func(v vm.VM) bool { return v.OwnerUUID == ownerUUID }, offset, limit)
}

func (r *Repository) GetAllByOwnerAndKind(_ context.Context, ownerUUID string, kind vm.Kind) ([]vm.VM, error) {
	return r.all(func(v vm.VM) bool { return v.OwnerUUID == ownerUUID && v.Kind == kind })
}

func (r *Repository) GetAllByNode(_ context.Context, nodeName string) ([]vm.VM, error) {
	return r.all(func(v vm.VM) bool { return v.NodeName == nodeName })
}

func (r *Repository) CountByOwner(_ context.Context, ownerUUID string) (uint, error) {
	items, err := r.all(func(v vm.VM) bool { return v.OwnerUUID == ownerUUID })

	return uint(len(items)), err
}

func (r *Repository) Count(_ context.Context) (uint, error) {
	items, err := r.all(func(vm.VM) bool { return true })

	return uint(len(items)), err
}

func (r *Repository) GetOne(_ context.Context, uuid string) (vm.VM, error) {
	return r.one(func(v vm.VM) bool { return v.UUID == uuid })
}

func (r *Repository) GetOneByOwner(_ context.Context, ownerUUID string, uuid string) (vm.VM, error) {
	return r.one(func(v vm.VM) bool { return v.UUID == uuid && v.OwnerUUID == ownerUUID })
}

func (r *Repository) GetOneBySlug(_ context.Context, slug string) (vm.VM, error) {
	return r.one(func(v vm.VM) bool { return v.Slug == slug })
}

func (r *Repository) Save(_ context.Context, v *vm.VM) (string, error) {
	if r.Fail != nil {
		return "", r.Fail
	}

	r.lock.Lock()
	defer r.lock.Unlock()

	if len(v.UUID) == 0 {
		id, err := uuid.NewV7()
		if err != nil {
			return "", err
		}

		v.UUID = id.String()
	}

	if v.CreatedAt.IsZero() {
		v.CreatedAt = time.Now()
	}

	stored, exists := r.vms[v.UUID]
	if exists && stored.UpdatedAt.After(v.UpdatedAt) {
		return "", ErrStale
	}

	for _, other := range r.vms {
		if other.UUID != v.UUID && len(v.Slug) > 0 && other.Slug == v.Slug {
			return "", domain.ErrAlreadyExists
		}
	}

	r.vms[v.UUID] = clone(*v)

	return v.UUID, nil
}

func (r *Repository) Delete(_ context.Context, uuid string) error {
	if r.Fail != nil {
		return r.Fail
	}

	r.lock.Lock()
	defer r.lock.Unlock()

	delete(r.vms, uuid)

	return nil
}

// Stored is the VM kept under uuid, for a test to look at.
func (r *Repository) Stored(uuid string) (vm.VM, bool) {
	r.lock.Lock()
	defer r.lock.Unlock()

	v, ok := r.vms[uuid]

	return clone(v), ok
}

// Len is how many VMs are kept.
func (r *Repository) Len() int {
	r.lock.Lock()
	defer r.lock.Unlock()

	return len(r.vms)
}

func (r *Repository) all(keep func(vm.VM) bool) ([]vm.VM, error) {
	if r.Fail != nil {
		return nil, r.Fail
	}

	r.lock.Lock()
	defer r.lock.Unlock()

	items := make([]vm.VM, 0, len(r.vms))
	for _, v := range r.vms {
		if keep(v) {
			items = append(items, clone(v))
		}
	}

	// newest first: uuids are v7, so they order by creation.
	slices.SortFunc(items, func(a, b vm.VM) int { return strings.Compare(b.UUID, a.UUID) })

	return items, nil
}

func (r *Repository) page(keep func(vm.VM) bool, offset uint, limit uint) ([]vm.VM, error) {
	items, err := r.all(keep)
	if err != nil {
		return nil, err
	}

	if offset >= uint(len(items)) {
		return []vm.VM{}, nil
	}

	return items[offset:min(offset+limit, uint(len(items)))], nil
}

func (r *Repository) one(match func(vm.VM) bool) (vm.VM, error) {
	items, err := r.all(match)
	if err != nil {
		return vm.VM{}, err
	}

	if len(items) == 0 {
		return vm.VM{}, domain.ErrNotExists
	}

	return items[0], nil
}

// clone keeps what is stored apart from what was handed in or out, as a
// database does.
func clone(v vm.VM) vm.VM {
	v.Ports = slices.Clone(v.Ports)

	return v
}
