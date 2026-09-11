package tunnel

import (
	"errors"
	"slices"
	"strings"
	"sync"
	"time"
)

// ErrNoSuchWorker is a worker with nothing connected.
var ErrNoSuchWorker = errors.New("tunnel: worker is not connected")

// WorkerState is what the ingress can say about a worker without holding
// anything: a snapshot, for reporting and for routing to decide on.
type WorkerState struct {
	Worker   string
	Sessions int
	Streams  int
	Capacity int

	// ConnectedAt is when the oldest connection still held was made, which is
	// as close to "when this worker came up" as the ingress can see.
	ConnectedAt time.Time
}

// Free is how many more streams the worker will take.
func (s WorkerState) Free() int {
	if s.Capacity <= s.Streams {
		return 0
	}

	return s.Capacity - s.Streams
}

// Load is the fraction of what it will carry that it is carrying, which is what
// makes two workers of different sizes comparable.
func (s WorkerState) Load() float64 {
	if s.Capacity == 0 {
		return 1
	}

	return float64(s.Streams) / float64(s.Capacity)
}

// Registry is which workers are connected and with what.
//
// It is behind an interface because it is the one thing an ingress cannot share
// today and will have to share tomorrow: a second ingress needs to know about
// workers connected to the first. Nothing above it knows whether the answer
// came from this process.
type Registry interface {
	// Add puts a session in, and reports the worker it now belongs to.
	Add(session *Session) error

	// Remove takes one out. A worker with none left stops existing.
	Remove(session *Session)

	// Sessions returns a worker's sessions. It reports ErrNoSuchWorker for one
	// that has none.
	Sessions(worker string) ([]*Session, error)

	// Workers reports every connected worker, ordered by name.
	Workers() []WorkerState
}

// memoryRegistry keeps the workers in memory, sharded by worker so that two
// workers registering at once do not wait for each other.
//
// The lock is held only to find or change the set of sessions, never while a
// stream is opened or a byte is copied: what is guarded is a map, not the data
// path.
type memoryRegistry struct {
	lock    sync.RWMutex
	workers map[string]*workerSessions
}

type workerSessions struct {
	lock     sync.RWMutex
	sessions []*Session
}

var _ Registry = &memoryRegistry{}

// NewRegistry returns a registry held in this process.
func NewRegistry() Registry {
	return &memoryRegistry{workers: make(map[string]*workerSessions)}
}

func (r *memoryRegistry) Add(session *Session) error {
	if len(session.Worker()) == 0 {
		return errors.New("tunnel: a session belongs to no worker")
	}

	r.lock.Lock()
	worker, ok := r.workers[session.Worker()]
	if !ok {
		worker = &workerSessions{}
		r.workers[session.Worker()] = worker
	}
	r.lock.Unlock()

	worker.lock.Lock()
	worker.sessions = append(worker.sessions, session)
	worker.lock.Unlock()

	return nil
}

func (r *memoryRegistry) Remove(session *Session) {
	r.lock.RLock()
	worker, ok := r.workers[session.Worker()]
	r.lock.RUnlock()

	if !ok {
		return
	}

	worker.lock.Lock()
	worker.sessions = slices.DeleteFunc(worker.sessions, func(held *Session) bool {
		return held == session
	})
	empty := len(worker.sessions) == 0
	worker.lock.Unlock()

	if !empty {
		return
	}

	// the last one went, so the worker is gone. It is checked again under the
	// write lock because one may have arrived in between, which is exactly what
	// a worker reconnecting looks like.
	r.lock.Lock()
	defer r.lock.Unlock()

	worker.lock.RLock()
	still := len(worker.sessions)
	worker.lock.RUnlock()

	if still == 0 {
		delete(r.workers, session.Worker())
	}
}

func (r *memoryRegistry) Sessions(name string) ([]*Session, error) {
	r.lock.RLock()
	worker, ok := r.workers[name]
	r.lock.RUnlock()

	if !ok {
		return nil, ErrNoSuchWorker
	}

	worker.lock.RLock()
	sessions := slices.Clone(worker.sessions)
	worker.lock.RUnlock()

	if len(sessions) == 0 {
		return nil, ErrNoSuchWorker
	}

	return sessions, nil
}

func (r *memoryRegistry) Workers() []WorkerState {
	r.lock.RLock()
	names := make([]string, 0, len(r.workers))
	held := make([]*workerSessions, 0, len(r.workers))
	for name, worker := range r.workers {
		names = append(names, name)
		held = append(held, worker)
	}
	r.lock.RUnlock()

	states := make([]WorkerState, 0, len(names))
	for i, worker := range held {
		worker.lock.RLock()
		sessions := slices.Clone(worker.sessions)
		worker.lock.RUnlock()

		state := WorkerState{Worker: names[i]}
		for _, session := range sessions {
			if session.Closed() {
				continue
			}

			state.Sessions++
			state.Streams += session.Streams()
			state.Capacity += session.Capacity()

			if state.ConnectedAt.IsZero() || session.CreatedAt().Before(state.ConnectedAt) {
				state.ConnectedAt = session.CreatedAt()
			}
		}

		if state.Sessions == 0 {
			continue
		}

		states = append(states, state)
	}

	slices.SortFunc(states, func(a WorkerState, b WorkerState) int {
		return strings.Compare(a.Worker, b.Worker)
	})

	return states
}
