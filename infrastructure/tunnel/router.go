package tunnel

import (
	"errors"
	"math"
)

// ErrNoAgentAvailable is every agent being gone, or full.
var ErrNoAgentAvailable = errors.New("tunnel: no agent available")

// Router picks the agent a client that named none will be bound to.
//
// The choice is made once, when the connection arrives, and holds for its whole
// life: a TCP connection cannot be moved to another agent afterwards, because
// neither end could be told that it had been.
type Router interface {
	Pick(agents []AgentState) (string, error)
}

// RouterFunc adapts a function to a Router.
type RouterFunc func(agents []AgentState) (string, error)

func (f RouterFunc) Pick(agents []AgentState) (string, error) { return f(agents) }

// LeastLoaded picks the agent carrying the smallest share of what it can
// carry, ignoring any that is full or has nothing connected.
//
// Load is measured in streams because that is what a session's capacity is
// counted in. It is a poor likeness of load — one stream moving a gigabyte and
// two hundred idle ssh sessions count one against two hundred — which is why
// this is behind an interface and why sessions also count bytes: a policy that
// weighs those can replace this one without anything else changing.
func LeastLoaded() Router {
	return RouterFunc(func(agents []AgentState) (string, error) {
		best := ""
		lowest := math.Inf(1)

		for _, agent := range agents {
			if agent.Sessions == 0 || agent.Free() == 0 {
				continue
			}

			if load := agent.Load(); load < lowest {
				best, lowest = agent.Name, load
			}
		}

		if len(best) == 0 {
			return "", ErrNoAgentAvailable
		}

		return best, nil
	})
}
