// Package executions keeps how each node of each root's plan went in memory,
// the way the MongoDB repository keeps it: one execution for a node of a
// root, by the root's uuid and the node, written over whole each time it is
// saved, a root's read in the order of their levels and then of their nodes,
// and times to the millisecond, in UTC.
package executions

import (
	"cmp"
	"context"
	"fmt"
	"slices"
	"sync"
	"time"

	"github.com/khanzadimahdi/testproject/domain/workload/graph"
)

type Repository struct {
	lock sync.Mutex

	// executions are what is kept, by their roots' uuids, and then by their
	// nodes.
	executions map[string]map[graph.NodeID]graph.Execution

	// Fail, when set, is what every call reports instead of doing anything.
	Fail error
}

var _ graph.Executions = &Repository{}

// NewRepository is a repository holding executions, as they are given. One
// that names no root's uuid or no node cannot be kept.
func NewRepository(executions ...graph.Execution) *Repository {
	r := &Repository{executions: make(map[string]map[graph.NodeID]graph.Execution)}

	for _, e := range executions {
		if err := r.Save(context.Background(), e); err != nil {
			panic(err)
		}
	}

	return r
}

func (r *Repository) Save(_ context.Context, e graph.Execution) error {
	if r.Fail != nil {
		return r.Fail
	}

	if len(e.Root.UUID) == 0 || len(e.Node) == 0 {
		return fmt.Errorf("an execution is kept by its root's uuid and its node, and that of %q in %q lacks one", e.Node, e.Root.UUID)
	}

	r.lock.Lock()
	defer r.lock.Unlock()

	if r.executions == nil {
		r.executions = make(map[string]map[graph.NodeID]graph.Execution)
	}

	if r.executions[e.Root.UUID] == nil {
		r.executions[e.Root.UUID] = make(map[graph.NodeID]graph.Execution)
	}

	r.executions[e.Root.UUID][e.Node] = kept(e)

	return nil
}

func (r *Repository) GetByRoot(_ context.Context, rootUUID string) ([]graph.Execution, error) {
	if r.Fail != nil {
		return nil, r.Fail
	}

	r.lock.Lock()
	defer r.lock.Unlock()

	executions := make([]graph.Execution, 0, len(r.executions[rootUUID]))
	for _, e := range r.executions[rootUUID] {
		executions = append(executions, e)
	}

	slices.SortFunc(executions, func(a, b graph.Execution) int {
		return cmp.Or(cmp.Compare(a.Level, b.Level), cmp.Compare(a.Node, b.Node))
	})

	return executions, nil
}

func (r *Repository) DeleteByRoot(_ context.Context, rootUUID string) error {
	if r.Fail != nil {
		return r.Fail
	}

	r.lock.Lock()
	defer r.lock.Unlock()

	delete(r.executions, rootUUID)

	return nil
}

func (r *Repository) DeleteBefore(_ context.Context, rootUUID string, generation int64) error {
	if r.Fail != nil {
		return r.Fail
	}

	r.lock.Lock()
	defer r.lock.Unlock()

	for node, e := range r.executions[rootUUID] {
		if e.Generation < generation {
			delete(r.executions[rootUUID], node)
		}
	}

	return nil
}

// kept is an execution as the database keeps it: its times to the
// millisecond, and in UTC.
func kept(e graph.Execution) graph.Execution {
	e.StartedAt = millisecond(e.StartedAt)
	e.FinishedAt = millisecond(e.FinishedAt)
	e.UpdatedAt = millisecond(e.UpdatedAt)

	return e
}

func millisecond(t time.Time) time.Time {
	if t.IsZero() {
		return time.Time{}
	}

	return t.UTC().Truncate(time.Millisecond)
}
