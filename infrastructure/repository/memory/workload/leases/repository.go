// Package leases keeps each root's lease in memory, the way the MongoDB
// repository keeps it: one lease for a root, by the root's uuid, taken when
// nobody holds it, it expired or its taker holds it, and its root is not
// backed off, as its clock judges, or taken now whatever backs it off, which
// it clears, and poked when another holds it; a failure in a row backing its
// root off twice as long as the last, and keeping what its run saw; and
// times to the millisecond, in UTC.
package leases

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/khanzadimahdi/testproject/domain"
	"github.com/khanzadimahdi/testproject/domain/workload/graph"
)

type Repository struct {
	lock sync.Mutex

	// leases are what is kept, by their roots' uuids, and now the clock
	// they are judged by.
	leases map[string]graph.Lease
	now    func() time.Time

	// Fail, when set, is what every call reports instead of doing anything.
	Fail error
}

var _ graph.Leases = &Repository{}

// NewRepository is a repository of leases judged by the clock now, holding
// none yet. A clock of nil is the time now.
func NewRepository(now func() time.Time) *Repository {
	if now == nil {
		now = time.Now
	}

	return &Repository{leases: make(map[string]graph.Lease), now: now}
}

func (r *Repository) Take(_ context.Context, rootUUID string, holder string, ttl time.Duration) (graph.Lease, error) {
	if r.Fail != nil {
		return graph.Lease{}, r.Fail
	}

	if err := asked(rootUUID, holder, ttl); err != nil {
		return graph.Lease{}, err
	}

	r.lock.Lock()
	defer r.lock.Unlock()

	now := r.clock()

	lease, kept := r.of()[rootUUID]
	if !kept {
		lease = graph.Lease{Root: rootUUID}
	}

	switch {
	case lease.NextAt.After(now):
		return lease, graph.ErrBackedOff
	case len(lease.Holder) > 0 && lease.Holder != holder && lease.ExpiresAt.After(now):
		lease.Poked = true
		r.leases[rootUUID] = lease

		return lease, graph.ErrHeld
	}

	lease.Holder = holder
	lease.ExpiresAt = now.Add(ttl.Truncate(time.Millisecond))
	lease.Poked = false
	lease.UpdatedAt = now

	r.leases[rootUUID] = lease

	return lease, nil
}

func (r *Repository) TakeNow(_ context.Context, rootUUID string, holder string, ttl time.Duration) (graph.Lease, error) {
	if r.Fail != nil {
		return graph.Lease{}, r.Fail
	}

	if err := asked(rootUUID, holder, ttl); err != nil {
		return graph.Lease{}, err
	}

	r.lock.Lock()
	defer r.lock.Unlock()

	now := r.clock()

	lease, kept := r.of()[rootUUID]
	if !kept {
		lease = graph.Lease{Root: rootUUID}
	}

	if len(lease.Holder) > 0 && lease.Holder != holder && lease.ExpiresAt.After(now) {
		lease.Poked = true
		r.leases[rootUUID] = lease

		return lease, graph.ErrHeld
	}

	lease.Holder = holder
	lease.ExpiresAt = now.Add(ttl.Truncate(time.Millisecond))
	lease.Failures = 0
	lease.Reason = ""
	lease.NextAt = time.Time{}
	lease.Seen = graph.Seen{}
	lease.Poked = false
	lease.UpdatedAt = now

	r.leases[rootUUID] = lease

	return lease, nil
}

func (r *Repository) Renew(_ context.Context, rootUUID string, holder string, ttl time.Duration) (graph.Lease, error) {
	if r.Fail != nil {
		return graph.Lease{}, r.Fail
	}

	if err := asked(rootUUID, holder, ttl); err != nil {
		return graph.Lease{}, err
	}

	r.lock.Lock()
	defer r.lock.Unlock()

	lease, kept := r.of()[rootUUID]
	if !kept || lease.Holder != holder {
		return graph.Lease{}, graph.ErrLost
	}

	now := r.clock()

	lease.ExpiresAt = now.Add(ttl.Truncate(time.Millisecond))
	lease.UpdatedAt = now

	r.leases[rootUUID] = lease

	return lease, nil
}

func (r *Repository) Release(_ context.Context, rootUUID string, holder string, ended graph.Ended) (graph.Lease, error) {
	if r.Fail != nil {
		return graph.Lease{}, r.Fail
	}

	if err := asked(rootUUID, holder, time.Millisecond); err != nil {
		return graph.Lease{}, err
	}

	if err := endOf(ended); err != nil {
		return graph.Lease{}, err
	}

	r.lock.Lock()
	defer r.lock.Unlock()

	lease, kept := r.of()[rootUUID]
	if !kept || lease.Holder != holder {
		return graph.Lease{}, graph.ErrLost
	}

	now := r.clock()

	lease.Holder = ""
	lease.ExpiresAt = time.Time{}
	lease.UpdatedAt = now

	switch ended.State {
	case graph.StateDone:
		lease.Failures = 0
		lease.Reason = ""
		lease.NextAt = time.Time{}
		lease.Seen = graph.Seen{}
	case graph.StateFailed:
		// worked out in milliseconds, as the database does.
		backoff := graph.Backoff{First: ended.Backoff.First.Truncate(time.Millisecond), Most: ended.Backoff.Most.Truncate(time.Millisecond)}

		lease.Failures++
		lease.Reason = ended.Reason
		lease.NextAt = now.Add(backoff.After(lease.Failures))
		lease.Seen = seen(ended.Seen)
	}

	r.leases[rootUUID] = lease

	return lease, nil
}

func (r *Repository) Forget(_ context.Context, rootUUID string) error {
	if r.Fail != nil {
		return r.Fail
	}

	r.lock.Lock()
	defer r.lock.Unlock()

	delete(r.of(), rootUUID)

	return nil
}

func (r *Repository) Get(_ context.Context, rootUUID string) (graph.Lease, error) {
	if r.Fail != nil {
		return graph.Lease{}, r.Fail
	}

	r.lock.Lock()
	defer r.lock.Unlock()

	lease, kept := r.of()[rootUUID]
	if !kept {
		return graph.Lease{}, domain.ErrNotExists
	}

	return lease, nil
}

// Lease is the lease kept of the root uuid names, and whether one is, for a
// test to look at.
func (r *Repository) Lease(rootUUID string) (graph.Lease, bool) {
	r.lock.Lock()
	defer r.lock.Unlock()

	lease, kept := r.of()[rootUUID]

	return lease, kept
}

// of is the leases kept, made the first time they are.
func (r *Repository) of() map[string]graph.Lease {
	if r.leases == nil {
		r.leases = make(map[string]graph.Lease)
	}

	return r.leases
}

// clock is the time now, as the leases are judged by it and kept: to the
// millisecond, in UTC.
func (r *Repository) clock() time.Time {
	now := r.now
	if now == nil {
		now = time.Now
	}

	return now().UTC().Truncate(time.Millisecond)
}

// seen is what a run saw as a lease keeps it: its time to the millisecond, in
// UTC, and none when it saw none.
func seen(s graph.Seen) graph.Seen {
	if len(s.State) == 0 {
		return graph.Seen{}
	}

	if !s.Since.IsZero() {
		s.Since = s.Since.UTC().Truncate(time.Millisecond)
	}

	return s
}

// asked is what is wrong with a lease of the root uuid names, for holder,
// for ttl: nothing, unless one of them is nothing, or ttl is shorter than a
// millisecond.
func asked(rootUUID string, holder string, ttl time.Duration) error {
	switch {
	case len(rootUUID) == 0:
		return errors.New("a lease is a root's, by its uuid, and this one names none")
	case len(holder) == 0:
		return errors.New("a lease is held by a control plane, and this one names none")
	case ttl < time.Millisecond:
		return fmt.Errorf("a lease is held for a millisecond at least, not %s", ttl)
	}

	return nil
}

// endOf is what is wrong with how a run ended: nothing, unless it is none of
// done, waiting and failed.
func endOf(ended graph.Ended) error {
	switch ended.State {
	case graph.StateDone, graph.StateWaiting, graph.StateFailed:
		return nil
	}

	return fmt.Errorf("a run ends done, waiting or failed, not %q", ended.State)
}
