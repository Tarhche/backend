// Package stacks keeps stacks in memory, the way the MongoDB repository keeps
// them: newest first, one slug per stack, and a copy read before a newer request
// is refused rather than written over it.
package stacks

import (
	"context"
	"errors"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/gofrs/uuid/v5"

	"github.com/khanzadimahdi/testproject/domain"
	"github.com/khanzadimahdi/testproject/domain/workload/stack"
)

// ErrStale is a copy of a stack written back after the stored one was asked
// for something it does not know of.
var ErrStale = errors.New("the stack was changed after this copy of it was read")

type Repository struct {
	lock   sync.Mutex
	stacks map[string]stack.Stack

	// Fail, when set, is what every call reports instead of doing anything.
	Fail error
}

var _ stack.Repository = &Repository{}

func NewRepository(stacks ...stack.Stack) *Repository {
	r := &Repository{stacks: make(map[string]stack.Stack, len(stacks))}

	for _, s := range stacks {
		r.stacks[s.UUID] = s
	}

	return r
}

func (r *Repository) GetAll(_ context.Context, offset uint, limit uint) ([]stack.Stack, error) {
	return r.page(func(stack.Stack) bool { return true }, offset, limit)
}

func (r *Repository) GetAllByOwner(_ context.Context, ownerUUID string, offset uint, limit uint) ([]stack.Stack, error) {
	return r.page(func(s stack.Stack) bool { return s.OwnerUUID == ownerUUID }, offset, limit)
}

func (r *Repository) GetAllByVM(_ context.Context, vmUUID string) ([]stack.Stack, error) {
	return r.all(func(s stack.Stack) bool { return s.VMUUID == vmUUID })
}

func (r *Repository) CountByOwner(_ context.Context, ownerUUID string) (uint, error) {
	items, err := r.all(func(s stack.Stack) bool { return s.OwnerUUID == ownerUUID })

	return uint(len(items)), err
}

func (r *Repository) Count(_ context.Context) (uint, error) {
	items, err := r.all(func(stack.Stack) bool { return true })

	return uint(len(items)), err
}

func (r *Repository) GetOne(_ context.Context, uuid string) (stack.Stack, error) {
	return r.one(func(s stack.Stack) bool { return s.UUID == uuid })
}

func (r *Repository) GetOneByOwner(_ context.Context, ownerUUID string, uuid string) (stack.Stack, error) {
	return r.one(func(s stack.Stack) bool { return s.UUID == uuid && s.OwnerUUID == ownerUUID })
}

func (r *Repository) GetOneBySlug(_ context.Context, slug string) (stack.Stack, error) {
	return r.one(func(s stack.Stack) bool { return s.Slug == slug })
}

func (r *Repository) Save(_ context.Context, s *stack.Stack) (string, error) {
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

	if stored, exists := r.stacks[s.UUID]; exists && stored.UpdatedAt.After(s.UpdatedAt) {
		return "", ErrStale
	}

	for _, other := range r.stacks {
		if other.UUID != s.UUID && len(s.Slug) > 0 && other.Slug == s.Slug {
			return "", domain.ErrAlreadyExists
		}
	}

	r.stacks[s.UUID] = *s

	return s.UUID, nil
}

func (r *Repository) Delete(_ context.Context, uuid string) error {
	if r.Fail != nil {
		return r.Fail
	}

	r.lock.Lock()
	defer r.lock.Unlock()

	delete(r.stacks, uuid)

	return nil
}

// Stored is the stack kept under uuid, for a test to look at.
func (r *Repository) Stored(uuid string) (stack.Stack, bool) {
	r.lock.Lock()
	defer r.lock.Unlock()

	s, ok := r.stacks[uuid]

	return s, ok
}

// Len is how many stacks are kept.
func (r *Repository) Len() int {
	r.lock.Lock()
	defer r.lock.Unlock()

	return len(r.stacks)
}

func (r *Repository) all(keep func(stack.Stack) bool) ([]stack.Stack, error) {
	if r.Fail != nil {
		return nil, r.Fail
	}

	r.lock.Lock()
	defer r.lock.Unlock()

	items := make([]stack.Stack, 0, len(r.stacks))
	for _, s := range r.stacks {
		if keep(s) {
			items = append(items, s)
		}
	}

	// newest first: uuids are v7, so they order by creation.
	slices.SortFunc(items, func(a, b stack.Stack) int { return strings.Compare(b.UUID, a.UUID) })

	return items, nil
}

func (r *Repository) page(keep func(stack.Stack) bool, offset uint, limit uint) ([]stack.Stack, error) {
	items, err := r.all(keep)
	if err != nil {
		return nil, err
	}

	if offset >= uint(len(items)) {
		return []stack.Stack{}, nil
	}

	return items[offset:min(offset+limit, uint(len(items)))], nil
}

func (r *Repository) one(match func(stack.Stack) bool) (stack.Stack, error) {
	items, err := r.all(match)
	if err != nil {
		return stack.Stack{}, err
	}

	if len(items) == 0 {
		return stack.Stack{}, domain.ErrNotExists
	}

	return items[0], nil
}
