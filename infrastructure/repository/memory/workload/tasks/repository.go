// Package tasks keeps tasks in memory, the way the MongoDB repository keeps
// them: newest first, and one slug per task.
package tasks

import (
	"context"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/gofrs/uuid/v5"

	"github.com/khanzadimahdi/testproject/domain"
	"github.com/khanzadimahdi/testproject/domain/workload/task"
)

type Repository struct {
	lock  sync.Mutex
	tasks map[string]task.Task

	// Fail, when set, is what every call reports instead of doing anything.
	Fail error
}

var _ task.Repository = &Repository{}

func NewRepository(tasks ...task.Task) *Repository {
	r := &Repository{tasks: make(map[string]task.Task, len(tasks))}

	for _, t := range tasks {
		r.tasks[t.UUID] = clone(t)
	}

	return r
}

func (r *Repository) GetAll(_ context.Context, offset uint, limit uint) ([]task.Task, error) {
	return r.page(func(task.Task) bool { return true }, offset, limit)
}

func (r *Repository) GetAllByOwner(_ context.Context, ownerUUID string, offset uint, limit uint) ([]task.Task, error) {
	return r.page(func(t task.Task) bool { return t.OwnerUUID == ownerUUID }, offset, limit)
}

func (r *Repository) CountByOwner(_ context.Context, ownerUUID string) (uint, error) {
	items, err := r.all(func(t task.Task) bool { return t.OwnerUUID == ownerUUID })

	return uint(len(items)), err
}

func (r *Repository) Count(_ context.Context) (uint, error) {
	items, err := r.all(func(task.Task) bool { return true })

	return uint(len(items)), err
}

func (r *Repository) GetOne(_ context.Context, uuid string) (task.Task, error) {
	return r.one(func(t task.Task) bool { return t.UUID == uuid })
}

func (r *Repository) GetOneByOwner(_ context.Context, ownerUUID string, uuid string) (task.Task, error) {
	return r.one(func(t task.Task) bool { return t.UUID == uuid && t.OwnerUUID == ownerUUID })
}

func (r *Repository) GetOneBySlug(_ context.Context, slug string) (task.Task, error) {
	return r.one(func(t task.Task) bool { return t.Slug == slug })
}

func (r *Repository) Save(_ context.Context, t *task.Task) (string, error) {
	if r.Fail != nil {
		return "", r.Fail
	}

	r.lock.Lock()
	defer r.lock.Unlock()

	if len(t.UUID) == 0 {
		id, err := uuid.NewV7()
		if err != nil {
			return "", err
		}

		t.UUID = id.String()
	}

	if t.CreatedAt.IsZero() {
		t.CreatedAt = time.Now()
	}

	for _, other := range r.tasks {
		if other.UUID != t.UUID && len(t.Slug) > 0 && other.Slug == t.Slug {
			return "", domain.ErrAlreadyExists
		}
	}

	r.tasks[t.UUID] = clone(*t)

	return t.UUID, nil
}

func (r *Repository) Delete(_ context.Context, uuid string) error {
	if r.Fail != nil {
		return r.Fail
	}

	r.lock.Lock()
	defer r.lock.Unlock()

	delete(r.tasks, uuid)

	return nil
}

// Stored is the task kept under uuid, for a test to look at.
func (r *Repository) Stored(uuid string) (task.Task, bool) {
	r.lock.Lock()
	defer r.lock.Unlock()

	t, ok := r.tasks[uuid]

	return clone(t), ok
}

func (r *Repository) all(keep func(task.Task) bool) ([]task.Task, error) {
	if r.Fail != nil {
		return nil, r.Fail
	}

	r.lock.Lock()
	defer r.lock.Unlock()

	items := make([]task.Task, 0, len(r.tasks))
	for _, t := range r.tasks {
		if keep(t) {
			items = append(items, clone(t))
		}
	}

	// newest first: uuids are v7, so they order by creation.
	slices.SortFunc(items, func(a, b task.Task) int { return strings.Compare(b.UUID, a.UUID) })

	return items, nil
}

func (r *Repository) page(keep func(task.Task) bool, offset uint, limit uint) ([]task.Task, error) {
	items, err := r.all(keep)
	if err != nil {
		return nil, err
	}

	if offset >= uint(len(items)) {
		return []task.Task{}, nil
	}

	return items[offset:min(offset+limit, uint(len(items)))], nil
}

func (r *Repository) one(match func(task.Task) bool) (task.Task, error) {
	items, err := r.all(match)
	if err != nil {
		return task.Task{}, err
	}

	if len(items) == 0 {
		return task.Task{}, domain.ErrNotExists
	}

	return items[0], nil
}

// clone keeps what is stored apart from what was handed in or out, as a
// database does.
func clone(t task.Task) task.Task {
	t.ExposedPorts = slices.Clone(t.ExposedPorts)
	t.PortBindings = slices.Clone(t.PortBindings)
	t.Endpoints = slices.Clone(t.Endpoints)
	t.Environment = slices.Clone(t.Environment)
	t.Command = slices.Clone(t.Command)
	t.Entrypoint = slices.Clone(t.Entrypoint)
	t.Mounts = slices.Clone(t.Mounts)
	t.ExecutionLogs = slices.Clone(t.ExecutionLogs)

	return t
}
