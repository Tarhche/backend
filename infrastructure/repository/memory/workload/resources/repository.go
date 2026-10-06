// Package resources keeps the resources of every kind in memory, the way the
// MongoDB repository keeps them: each kind apart, one slug per resource of a
// kind, newest first, a copy written back only over the version it was read
// at, and times to the millisecond, in UTC.
package resources

import (
	"context"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/gofrs/uuid/v5"

	"github.com/khanzadimahdi/testproject/domain"
	"github.com/khanzadimahdi/testproject/domain/workload/resource"
)

type Repository struct {
	lock sync.Mutex

	// kinds are the resources kept, by kind and then by uuid.
	kinds map[string]map[string]resource.Record

	// Fail, when set, is what every call reports instead of doing anything.
	Fail error
}

var _ resource.Repository = &Repository{}

// NewRepository is a repository holding records, as they are given.
func NewRepository(records ...resource.Record) *Repository {
	r := &Repository{kinds: make(map[string]map[string]resource.Record)}

	for _, record := range records {
		r.of(record.Kind)[record.Metadata.UUID] = kept(record)
	}

	return r
}

func (r *Repository) Create(_ context.Context, record resource.Record) (resource.Record, error) {
	if r.Fail != nil {
		return resource.Record{}, r.Fail
	}

	r.lock.Lock()
	defer r.lock.Unlock()

	if len(record.Metadata.UUID) == 0 {
		id, err := uuid.NewV7()
		if err != nil {
			return resource.Record{}, err
		}

		record.Metadata.UUID = id.String()
	}

	stored := r.of(record.Kind)

	if _, taken := stored[record.Metadata.UUID]; taken || r.slugTaken(record) {
		return resource.Record{}, domain.ErrAlreadyExists
	}

	record.Version = 1
	record = kept(record)
	stored[record.Metadata.UUID] = record

	return record.Clone(), nil
}

func (r *Repository) Update(_ context.Context, record resource.Record) (resource.Record, error) {
	if r.Fail != nil {
		return resource.Record{}, r.Fail
	}

	r.lock.Lock()
	defer r.lock.Unlock()

	stored := r.of(record.Kind)

	current, exists := stored[record.Metadata.UUID]
	switch {
	case !exists:
		return resource.Record{}, domain.ErrNotExists
	case current.Version != record.Version:
		return resource.Record{}, resource.ErrConflict
	case r.slugTaken(record):
		return resource.Record{}, domain.ErrAlreadyExists
	}

	record.Version++
	record = kept(record)
	stored[record.Metadata.UUID] = record

	return record.Clone(), nil
}

func (r *Repository) Delete(_ context.Context, kindName string, uuid string) error {
	if r.Fail != nil {
		return r.Fail
	}

	r.lock.Lock()
	defer r.lock.Unlock()

	delete(r.of(kindName), uuid)

	return nil
}

func (r *Repository) GetOne(_ context.Context, kindName string, uuid string) (resource.Record, error) {
	return r.one(kindName, func(record resource.Record) bool {
		return record.Metadata.UUID == uuid
	})
}

func (r *Repository) GetOneByOwner(_ context.Context, kindName string, ownerUUID string, uuid string) (resource.Record, error) {
	return r.one(kindName, func(record resource.Record) bool {
		return record.Metadata.UUID == uuid && record.Metadata.OwnerUUID == ownerUUID
	})
}

func (r *Repository) GetOneBySlug(_ context.Context, kindName string, slug string) (resource.Record, error) {
	return r.one(kindName, func(record resource.Record) bool {
		return len(slug) > 0 && record.Metadata.Slug == slug
	})
}

func (r *Repository) GetAll(_ context.Context, kindName string, filter resource.Filter, offset uint, limit uint) ([]resource.Record, uint, error) {
	if r.Fail != nil {
		return nil, 0, r.Fail
	}

	r.lock.Lock()
	defer r.lock.Unlock()

	var matching []resource.Record
	for _, record := range r.of(kindName) {
		if passes(record, filter) {
			matching = append(matching, record)
		}
	}

	// newest first, by uuid, as a v7 orders by when it was made.
	slices.SortFunc(matching, func(a, b resource.Record) int {
		return strings.Compare(b.Metadata.UUID, a.Metadata.UUID)
	})

	total := uint(len(matching))

	page := make([]resource.Record, 0)
	if offset >= total {
		return page, total, nil
	}

	end := total
	if limit > 0 {
		end = min(offset+limit, total)
	}

	for _, record := range matching[offset:end] {
		page = append(page, record.Clone())
	}

	return page, total, nil
}

// Len is how many resources of the kind are kept, for a test to look at.
func (r *Repository) Len(kindName string) int {
	r.lock.Lock()
	defer r.lock.Unlock()

	return len(r.kinds[kindName])
}

// Stored is the resource of the kind kept under uuid, for a test to look at.
func (r *Repository) Stored(kindName string, uuid string) (resource.Record, bool) {
	r.lock.Lock()
	defer r.lock.Unlock()

	record, ok := r.kinds[kindName][uuid]

	return record.Clone(), ok
}

func (r *Repository) one(kindName string, matches func(resource.Record) bool) (resource.Record, error) {
	if r.Fail != nil {
		return resource.Record{}, r.Fail
	}

	r.lock.Lock()
	defer r.lock.Unlock()

	for _, record := range r.of(kindName) {
		if matches(record) {
			return record.Clone(), nil
		}
	}

	return resource.Record{}, domain.ErrNotExists
}

// of is the resources of one kind, made the first time the kind is.
func (r *Repository) of(kindName string) map[string]resource.Record {
	if r.kinds == nil {
		r.kinds = make(map[string]map[string]resource.Record)
	}

	stored, ok := r.kinds[kindName]
	if !ok {
		stored = make(map[string]resource.Record)
		r.kinds[kindName] = stored
	}

	return stored
}

// slugTaken reports whether another resource of the record's kind has its
// slug. A resource reached under no name shares none.
func (r *Repository) slugTaken(record resource.Record) bool {
	if len(record.Metadata.Slug) == 0 {
		return false
	}

	for uuid, other := range r.of(record.Kind) {
		if uuid != record.Metadata.UUID && other.Metadata.Slug == record.Metadata.Slug {
			return true
		}
	}

	return false
}

// passes reports whether a record is one filter lets through.
func passes(record resource.Record, filter resource.Filter) bool {
	return filter.Passes(record.Metadata)
}

// kept is a record as the database keeps it: a copy of its own, its times to
// the millisecond and in UTC.
func kept(record resource.Record) resource.Record {
	record = record.Clone()

	record.Metadata.ExpiresAt = millisecond(record.Metadata.ExpiresAt)
	record.Metadata.CreatedAt = millisecond(record.Metadata.CreatedAt)
	record.Metadata.UpdatedAt = millisecond(record.Metadata.UpdatedAt)
	record.TriedAt = millisecond(record.TriedAt)

	if record.Pending != nil {
		record.Pending.SentAt = millisecond(record.Pending.SentAt)
	}

	if record.Answer != nil {
		record.Answer.At = millisecond(record.Answer.At)
		record.Answer.Status = nil
	}

	return record
}

func millisecond(t time.Time) time.Time {
	if t.IsZero() {
		return time.Time{}
	}

	return t.UTC().Truncate(time.Millisecond)
}
