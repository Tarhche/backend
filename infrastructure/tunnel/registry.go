package tunnel

import (
	"errors"
	"slices"
	"strings"
	"sync"
)

// ErrNoSuchAgent is an agent with nothing connected.
var ErrNoSuchAgent = errors.New("tunnel: agent is not connected")

// AgentState is what the hub can say about an agent without holding
// anything: a snapshot, for reporting and for routing to decide on.
type AgentState struct {
	Name     string
	Sessions int
	Streams  int
	Capacity int
}

// Free is how many more streams the agent will take.
func (s AgentState) Free() int {
	if s.Capacity <= s.Streams {
		return 0
	}

	return s.Capacity - s.Streams
}

// Load is the fraction of what it will carry that it is carrying, which is what
// makes two agents of different sizes comparable.
func (s AgentState) Load() float64 {
	if s.Capacity == 0 {
		return 1
	}

	return float64(s.Streams) / float64(s.Capacity)
}

// Registry is which agents are connected and with what.
//
// It is behind an interface because it is the one thing a hub cannot share
// today and will have to share tomorrow: a second hub needs to know about
// agents connected to the first. Nothing above it knows whether the answer
// came from this process.
type Registry interface {
	// Add puts a session in, and reports the agent it now belongs to.
	Add(session *Session) error

	// Remove takes one out. An agent with none left stops existing.
	Remove(session *Session)

	// Sessions returns an agent's sessions. It reports ErrNoSuchAgent for one
	// that has none.
	Sessions(agent string) ([]*Session, error)

	// Agents reports every connected agent, ordered by name.
	Agents() []AgentState
}

// memoryRegistry keeps the agents in memory.
//
// One lock, held for writing only while the set of sessions changes — which
// happens once per connection — and for reading on the path of every client
// connection. Sharding it per agent was tempting and wrong: a session being
// added while another agent's last one is removed has to see a consistent map,
// and two locks taken in sequence do not give that. Registration is rare enough
// that it does not need to be concurrent; routing is not, and only reads.
type memoryRegistry struct {
	lock   sync.RWMutex
	agents map[string][]*Session
}

var _ Registry = &memoryRegistry{}

// NewRegistry returns a registry held in this process.
func NewRegistry() Registry {
	return &memoryRegistry{agents: make(map[string][]*Session)}
}

func (r *memoryRegistry) Add(session *Session) error {
	if len(session.Agent()) == 0 {
		return errors.New("tunnel: a session belongs to no agent")
	}

	r.lock.Lock()
	defer r.lock.Unlock()

	r.agents[session.Agent()] = append(r.agents[session.Agent()], session)

	return nil
}

func (r *memoryRegistry) Remove(session *Session) {
	r.lock.Lock()
	defer r.lock.Unlock()

	sessions, ok := r.agents[session.Agent()]
	if !ok {
		return
	}

	sessions = slices.DeleteFunc(sessions, func(held *Session) bool {
		return held == session
	})

	// the last one went, so the agent is gone. Nothing can have arrived in
	// between: adding takes this same lock.
	if len(sessions) == 0 {
		delete(r.agents, session.Agent())

		return
	}

	r.agents[session.Agent()] = sessions
}

func (r *memoryRegistry) Sessions(name string) ([]*Session, error) {
	r.lock.RLock()
	defer r.lock.RUnlock()

	sessions, ok := r.agents[name]
	if !ok || len(sessions) == 0 {
		return nil, ErrNoSuchAgent
	}

	return slices.Clone(sessions), nil
}

func (r *memoryRegistry) Agents() []AgentState {
	r.lock.RLock()
	held := make(map[string][]*Session, len(r.agents))
	for name, sessions := range r.agents {
		held[name] = slices.Clone(sessions)
	}
	r.lock.RUnlock()

	states := make([]AgentState, 0, len(held))
	for name, sessions := range held {
		state := AgentState{Name: name}
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

	slices.SortFunc(states, func(a AgentState, b AgentState) int {
		return strings.Compare(a.Name, b.Name)
	})

	return states
}
