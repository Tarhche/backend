package logs

import (
	"context"
	"sync"
	"time"

	"github.com/khanzadimahdi/testproject/domain/runner/container"
)

// InMemoryLogRepository keeps a container's lines in memory, identifying each
// one the way the real repository does — by its own content — so that storing
// the same line twice is a no-op here as well.
type InMemoryLogRepository struct {
	lock  sync.Mutex
	lines map[string][]container.Log
	seen  map[string]struct{}

	// Fail, when set, is what every call reports instead of doing anything.
	Fail error
}

var _ container.LogRepository = &InMemoryLogRepository{}

func NewInMemoryRepository() *InMemoryLogRepository {
	return &InMemoryLogRepository{
		lines: make(map[string][]container.Log),
		seen:  make(map[string]struct{}),
	}
}

func (r *InMemoryLogRepository) Append(_ context.Context, logs []container.Log) error {
	if r.Fail != nil {
		return r.Fail
	}

	r.lock.Lock()
	defer r.lock.Unlock()

	for _, l := range logs {
		id := l.TaskUUID + "|" + l.At.UTC().Format(time.RFC3339Nano) + "|" + l.Stream.String() + "|" + l.Content
		if _, duplicate := r.seen[id]; duplicate {
			continue
		}

		r.seen[id] = struct{}{}
		r.lines[l.TaskUUID] = append(r.lines[l.TaskUUID], l)
	}

	return nil
}

func (r *InMemoryLogRepository) Get(_ context.Context, taskUUID string, after time.Time, limit uint) ([]container.Log, error) {
	if r.Fail != nil {
		return nil, r.Fail
	}

	r.lock.Lock()
	defer r.lock.Unlock()

	found := make([]container.Log, 0, len(r.lines[taskUUID]))
	for _, l := range r.lines[taskUUID] {
		if !after.IsZero() && !l.At.After(after) {
			continue
		}

		found = append(found, l)

		if limit > 0 && uint(len(found)) == limit {
			break
		}
	}

	return found, nil
}

func (r *InMemoryLogRepository) DeleteByTask(_ context.Context, taskUUID string) error {
	if r.Fail != nil {
		return r.Fail
	}

	r.lock.Lock()
	defer r.lock.Unlock()

	delete(r.lines, taskUUID)

	return nil
}

// Size reports how many bytes a task has stored, which is what the manager caps
// a chatty container against.
func (r *InMemoryLogRepository) Size(_ context.Context, taskUUID string) (int64, error) {
	if r.Fail != nil {
		return 0, r.Fail
	}

	r.lock.Lock()
	defer r.lock.Unlock()

	var bytes int64
	for _, l := range r.lines[taskUUID] {
		bytes += int64(len(l.Content))
	}

	return bytes, nil
}

// Count reports how many lines a task has stored.
func (r *InMemoryLogRepository) Count(taskUUID string) int {
	r.lock.Lock()
	defer r.lock.Unlock()

	return len(r.lines[taskUUID])
}
