package tunnel

import (
	"errors"
	"math"
)

// ErrNoWorkerAvailable is every worker being gone, or full.
var ErrNoWorkerAvailable = errors.New("tunnel: no worker available")

// Router picks the worker a client that named none will be bound to.
//
// The choice is made once, when the connection arrives, and holds for its whole
// life: a TCP connection cannot be moved to another worker afterwards, because
// neither end could be told that it had been.
type Router interface {
	Pick(workers []WorkerState) (string, error)
}

// RouterFunc adapts a function to a Router.
type RouterFunc func(workers []WorkerState) (string, error)

func (f RouterFunc) Pick(workers []WorkerState) (string, error) { return f(workers) }

// LeastLoaded picks the worker carrying the smallest share of what it can
// carry, ignoring any that is full or has nothing connected.
//
// Load is measured in streams because that is what a session's capacity is
// counted in. It is a poor likeness of load — one stream moving a gigabyte and
// two hundred idle ssh sessions count one against two hundred — which is why
// this is behind an interface and why sessions also count bytes: a policy that
// weighs those can replace this one without anything else changing.
func LeastLoaded() Router {
	return RouterFunc(func(workers []WorkerState) (string, error) {
		best := ""
		lowest := math.Inf(1)

		for _, worker := range workers {
			if worker.Sessions == 0 || worker.Free() == 0 {
				continue
			}

			if load := worker.Load(); load < lowest {
				best, lowest = worker.Worker, load
			}
		}

		if len(best) == 0 {
			return "", ErrNoWorkerAvailable
		}

		return best, nil
	})
}
