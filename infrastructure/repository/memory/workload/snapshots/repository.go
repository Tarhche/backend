// Package snapshots keeps snapshot records in memory, the way the MongoDB
// repository keeps them: newest first.
package snapshots

import (
	"context"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/gofrs/uuid/v5"

	"github.com/khanzadimahdi/testproject/domain"
	"github.com/khanzadimahdi/testproject/domain/workload/snapshot"
)

type Repository struct {
	lock      sync.Mutex
	snapshots map[string]snapshot.Snapshot

	// Fail, when set, is what every call reports instead of doing anything.
	Fail error
}

var _ snapshot.Repository = &Repository{}

func NewRepository(snapshots ...snapshot.Snapshot) *Repository {
	r := &Repository{snapshots: make(map[string]snapshot.Snapshot, len(snapshots))}

	for _, s := range snapshots {
		r.snapshots[s.UUID] = s
	}

	return r
}

func (r *Repository) GetAll(_ context.Context, offset uint, limit uint) ([]snapshot.Snapshot, error) {
	return r.page(func(snapshot.Snapshot) bool { return true }, offset, limit)
}

func (r *Repository) GetAllByOwner(_ context.Context, ownerUUID string, offset uint, limit uint) ([]snapshot.Snapshot, error) {
	return r.page(func(s snapshot.Snapshot) bool { return s.OwnerUUID == ownerUUID }, offset, limit)
}

func (r *Repository) GetAllByVM(_ context.Context, vmUUID string) ([]snapshot.Snapshot, error) {
	return r.all(func(s snapshot.Snapshot) bool { return s.VMUUID == vmUUID })
}

func (r *Repository) CountByOwner(_ context.Context, ownerUUID string) (uint, error) {
	items, err := r.all(func(s snapshot.Snapshot) bool { return s.OwnerUUID == ownerUUID })

	return uint(len(items)), err
}

func (r *Repository) Count(_ context.Context) (uint, error) {
	items, err := r.all(func(snapshot.Snapshot) bool { return true })

	return uint(len(items)), err
}

func (r *Repository) GetOne(_ context.Context, uuid string) (snapshot.Snapshot, error) {
	return r.one(func(s snapshot.Snapshot) bool { return s.UUID == uuid })
}

func (r *Repository) GetOneByOwner(_ context.Context, ownerUUID string, uuid string) (snapshot.Snapshot, error) {
	return r.one(func(s snapshot.Snapshot) bool { return s.UUID == uuid && s.OwnerUUID == ownerUUID })
}

func (r *Repository) Save(_ context.Context, s *snapshot.Snapshot) (string, error) {
	if r.Fail != nil {
		return "", r.Fail
	}

	r.lock.Lock()
	defer r.lock.Unlock()

	if len(s.UUID) == 0 {
		id, err := uuid.NewV7()
		if err != nil {
			return "", err
		}

		s.UUID = id.String()
	}

	if s.CreatedAt.IsZero() {
		s.CreatedAt = time.Now()
	}

	r.snapshots[s.UUID] = *s

	return s.UUID, nil
}

func (r *Repository) Delete(_ context.Context, uuid string) error {
	if r.Fail != nil {
		return r.Fail
	}

	r.lock.Lock()
	defer r.lock.Unlock()

	delete(r.snapshots, uuid)

	return nil
}

// Stored is the snapshot kept under uuid, for a test to look at.
func (r *Repository) Stored(uuid string) (snapshot.Snapshot, bool) {
	r.lock.Lock()
	defer r.lock.Unlock()

	s, ok := r.snapshots[uuid]

	return s, ok
}

func (r *Repository) all(keep func(snapshot.Snapshot) bool) ([]snapshot.Snapshot, error) {
	if r.Fail != nil {
		return nil, r.Fail
	}

	r.lock.Lock()
	defer r.lock.Unlock()

	items := make([]snapshot.Snapshot, 0, len(r.snapshots))
	for _, s := range r.snapshots {
		if keep(s) {
			items = append(items, s)
		}
	}

	// newest first: uuids are v7, so they order by creation.
	slices.SortFunc(items, func(a, b snapshot.Snapshot) int { return strings.Compare(b.UUID, a.UUID) })

	return items, nil
}

func (r *Repository) page(keep func(snapshot.Snapshot) bool, offset uint, limit uint) ([]snapshot.Snapshot, error) {
	items, err := r.all(keep)
	if err != nil {
		return nil, err
	}

	if offset >= uint(len(items)) {
		return []snapshot.Snapshot{}, nil
	}

	return items[offset:min(offset+limit, uint(len(items)))], nil
}

func (r *Repository) one(match func(snapshot.Snapshot) bool) (snapshot.Snapshot, error) {
	items, err := r.all(match)
	if err != nil {
		return snapshot.Snapshot{}, err
	}

	if len(items) == 0 {
		return snapshot.Snapshot{}, domain.ErrNotExists
	}

	return items[0], nil
}
