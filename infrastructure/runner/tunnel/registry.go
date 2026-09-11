package tunnel

import (
	"errors"
	"slices"
	"strings"
	"sync"
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

// memoryRegistry keeps the workers in memory.
//
// One lock, held for writing only while the set of sessions changes — which
// happens once per connection — and for reading on the path of every client
// connection. Sharding it per worker was tempting and wrong: a session being
// added while another worker's last one is removed has to see a consistent map,
// and two locks taken in sequence do not give that. Registration is rare enough
// that it does not need to be concurrent; routing is not, and only reads.
type memoryRegistry struct {
	lock    sync.RWMutex
	workers map[string][]*Session
}

var _ Registry = &memoryRegistry{}

// NewRegistry returns a registry held in this process.
func NewRegistry() Registry {
	return &memoryRegistry{workers: make(map[string][]*Session)}
}

func (r *memoryRegistry) Add(session *Session) error {
	if len(session.Worker()) == 0 {
		return errors.New("tunnel: a session belongs to no worker")
	}

	r.lock.Lock()
	defer r.lock.Unlock()

	r.workers[session.Worker()] = append(r.workers[session.Worker()], session)

	return nil
}

func (r *memoryRegistry) Remove(session *Session) {
	r.lock.Lock()
	defer r.lock.Unlock()

	sessions, ok := r.workers[session.Worker()]
	if !ok {
		return
	}

	sessions = slices.DeleteFunc(sessions, func(held *Session) bool {
		return held == session
	})

	// the last one went, so the worker is gone. Nothing can have arrived in
	// between: adding takes this same lock.
	if len(sessions) == 0 {
		delete(r.workers, session.Worker())

		return
	}

	r.workers[session.Worker()] = sessions
}

func (r *memoryRegistry) Sessions(name string) ([]*Session, error) {
	r.lock.RLock()
	defer r.lock.RUnlock()

	sessions, ok := r.workers[name]
	if !ok || len(sessions) == 0 {
		return nil, ErrNoSuchWorker
	}

	return slices.Clone(sessions), nil
}

func (r *memoryRegistry) Workers() []WorkerState {
	r.lock.RLock()
	held := make(map[string][]*Session, len(r.workers))
	for name, sessions := range r.workers {
		held[name] = slices.Clone(sessions)
	}
	r.lock.RUnlock()

	states := make([]WorkerState, 0, len(held))
	for name, sessions := range held {
		state := WorkerState{Worker: name}
		for _, session := range sessions {
			if session.Closed() {
				continue
			}

			state.Sessions++
			state.Streams += session.Streams()
			state.Capacity += session.Capacity()
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
