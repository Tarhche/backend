// Package resources keeps the resources of every kind in memory, the way the
// MongoDB repository keeps them: every kind's together, each read, written
// and deleted as one of its kind, a uuid and a slug one resource's of
// whatever kind, newest first, a copy written back only over the version it
// was read at, and times to the millisecond, in UTC.
package resources

import (
	"context"
	"fmt"
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

	// records are the resources kept, of every kind, by uuid.
	records map[string]resource.Record

	// Fail, when set, is what every call reports instead of doing anything.
	Fail error
}

var _ resource.Repository = &Repository{}

// NewRepository is a repository holding records, as they are given. Two of
// them under one uuid cannot both be kept, whatever their kinds.
func NewRepository(records ...resource.Record) *Repository {
	r := &Repository{records: make(map[string]resource.Record, len(records))}

	for _, record := range records {
		if other, taken := r.records[record.Metadata.UUID]; taken {
			panic(fmt.Sprintf("a %s and a %s cannot both be kept under the uuid %q", other.Kind, record.Kind, record.Metadata.UUID))
		}

		r.records[record.Metadata.UUID] = kept(record)
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

	if _, taken := r.of()[record.Metadata.UUID]; taken || r.slugTaken(record) {
		return resource.Record{}, domain.ErrAlreadyExists
	}

	record.Version = 1
	record = kept(record)
	r.records[record.Metadata.UUID] = record

	return record.Clone(), nil
}

func (r *Repository) Update(_ context.Context, record resource.Record) (resource.Record, error) {
	if r.Fail != nil {
		return resource.Record{}, r.Fail
	}

	r.lock.Lock()
	defer r.lock.Unlock()

	current, exists := r.of()[record.Metadata.UUID]
	switch {
	case !exists || current.Kind != record.Kind:
		return resource.Record{}, domain.ErrNotExists
	case current.Version != record.Version:
		return resource.Record{}, resource.ErrConflict
	case r.slugTaken(record):
		return resource.Record{}, domain.ErrAlreadyExists
	}

	record.Version++
	record = kept(record)
	r.records[record.Metadata.UUID] = record

	return record.Clone(), nil
}

func (r *Repository) Delete(_ context.Context, kindName string, uuid string) error {
	if r.Fail != nil {
		return r.Fail
	}

	r.lock.Lock()
	defer r.lock.Unlock()

	if current, exists := r.of()[uuid]; exists && current.Kind == kindName {
		delete(r.records, uuid)
	}

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
	for _, record := range r.of() {
		if record.Kind == kindName && passes(record, filter) {
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

	count := 0
	for _, record := range r.records {
		if record.Kind == kindName {
			count++
		}
	}

	return count
}

// Stored is the resource of the kind kept under uuid, for a test to look at.
func (r *Repository) Stored(kindName string, uuid string) (resource.Record, bool) {
	r.lock.Lock()
	defer r.lock.Unlock()

	record, ok := r.records[uuid]
	if !ok || record.Kind != kindName {
		return resource.Record{}, false
	}

	return record.Clone(), true
}

func (r *Repository) one(kindName string, matches func(resource.Record) bool) (resource.Record, error) {
	if r.Fail != nil {
		return resource.Record{}, r.Fail
	}

	r.lock.Lock()
	defer r.lock.Unlock()

	for _, record := range r.of() {
		if record.Kind == kindName && matches(record) {
			return record.Clone(), nil
		}
	}

	return resource.Record{}, domain.ErrNotExists
}

// of is the resources kept, of every kind, made the first time they are.
func (r *Repository) of() map[string]resource.Record {
	if r.records == nil {
		r.records = make(map[string]resource.Record)
	}

	return r.records
}

// slugTaken reports whether another resource, of whatever kind, has the
// record's slug. A resource reached under no name shares none.
func (r *Repository) slugTaken(record resource.Record) bool {
	if len(record.Metadata.Slug) == 0 {
		return false
	}

	for uuid, other := range r.of() {
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
