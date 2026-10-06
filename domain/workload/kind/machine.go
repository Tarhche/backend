package kind

import (
	"fmt"
	"slices"
)

// Any stands for every state as a transition's From: an action that may be
// taken from anywhere, such as delete, or an observation believed whatever
// was recorded. A transition from the state itself goes before one from Any,
// and nothing leaves Deleted, not even from Any.
const Any State = "*"

// Trigger is what moves a resource: an action asked of it, or a state its
// node was observed in. It is one or the other, never both.
type Trigger struct {
	Action   string `json:"action,omitempty"`
	Observed State  `json:"observed,omitempty"`
}

// OnAction is the trigger of asking for an action.
func OnAction(name string) Trigger {
	return Trigger{Action: name}
}

// OnObserved is the trigger of a node, or a command's result, saying a
// resource is in state s. Missing is observed of one its node no longer
// holds.
func OnObserved(s State) Trigger {
	return Trigger{Observed: s}
}

func (t Trigger) String() string {
	if len(t.Action) > 0 {
		return "action " + t.Action
	}

	return "observing " + string(t.Observed)
}

// valid reports whether t is an action or an observation, and not both.
func (t Trigger) valid() bool {
	return (len(t.Action) > 0) != (len(t.Observed) > 0)
}

// Transition is one move: from a state, on a trigger, to another state.
type Transition struct {
	From State   `json:"from"`
	On   Trigger `json:"on"`
	To   State   `json:"to"`
}

// Machine is a kind's states and the moves between them.
//
// A state is of one of three sorts. An in-flight state is one a resource
// passes through while something asked of it has yet to happen: starting,
// stopping, deleting. A terminal state is one it has ended in, where nothing
// happens until it is asked for something again: stopped, failed, deleted.
// Every other state is one it lives in, running or degraded, which is neither.
// The difference matters to the framework: an in-flight state is waited on
// rather than acted on, and a node falling silent fails a resource that is
// living but changes nothing about one that has ended.
//
// What a resource is asked moves it only where a transition says, and leaves
// it where it is otherwise. What it is observed doing moves it where a
// transition says, and then it depends on where it is: a resource at rest is
// whatever its node says it is, while an in-flight one believes only the
// arrivals its transitions declare, since a report sent before the command
// reached the node still says what it was doing before. A stop is not undone
// by the heartbeat that was already on its way.
type Machine struct {
	// Initial is the state an admitted resource starts in.
	Initial State `json:"initial"`

	States      []State      `json:"states"`
	Transitions []Transition `json:"transitions"`

	// Terminal are the states a resource ends in; InFlight the states it
	// passes through. A state is at most one of the two.
	Terminal []State `json:"terminal"`
	InFlight []State `json:"in_flight"`
}

// Has reports whether s is one of the machine's states.
func (m Machine) Has(s State) bool {
	return slices.Contains(m.States, s)
}

// IsTerminal reports whether a resource in s has ended.
func (m Machine) IsTerminal(s State) bool {
	return slices.Contains(m.Terminal, s)
}

// IsInFlight reports whether a resource in s is on its way somewhere, which
// is not the same as being somewhere it should not be.
func (m Machine) IsInFlight(s State) bool {
	return slices.Contains(m.InFlight, s)
}

// Next is the state a trigger takes a resource recorded in from to, and
// whether the machine takes the trigger at all. When it does not, the state
// it gives is from: the resource stays where it is.
//
// An action is taken only where a transition says, and one with no
// transition from where the resource is moves nothing. Whether the action may
// be asked there at all is the action's to say (Descriptor.Allows), not the
// machine's: a snapshot is taken of a running VM, which stays running.
//
// An observation is taken where a transition says, and otherwise when it is
// of the state the resource is in already, or when the resource is not in
// flight and the machine has the state observed. Nothing at all is taken
// from Deleted, or from a state the machine does not have.
func (m Machine) Next(from State, on Trigger) (State, bool) {
	if !on.valid() || !m.Has(from) || from == Deleted {
		return from, false
	}

	if to, declared := m.declared(from, on); declared {
		return to, true
	}

	if len(on.Action) > 0 {
		return from, false
	}

	switch observed := on.Observed; {
	case observed == from:
		return from, true
	case m.IsInFlight(from):
		return from, false
	case m.Has(observed):
		return observed, true
	}

	return from, false
}

// declared is where the transition on a trigger from a state leads, if one
// is declared: from the state itself, or else from Any.
func (m Machine) declared(from State, on Trigger) (State, bool) {
	var (
		to    State
		found bool
	)

	for _, t := range m.Transitions {
		if t.On != on {
			continue
		}

		switch t.From {
		case from:
			return t.To, true
		case Any:
			to, found = t.To, true
		}
	}

	return to, found
}

// leaving are the transitions a resource in s may take: those from s, and
// those from Any unless s is Deleted, which nothing leaves.
func (m Machine) leaving(s State) []Transition {
	var leaving []Transition

	for _, t := range m.Transitions {
		if t.From == s || (t.From == Any && s != Deleted) {
			leaving = append(leaving, t)
		}
	}

	return leaving
}

// Validate is everything about the machine that does not hold together, or
// nothing when it does: states listed twice or not at all, transitions to or
// from nowhere, two transitions on one trigger from one state, an in-flight
// state with no way out, and a state no transition reaches.
//
// A state is reached when a path of declared transitions leads to it from
// the initial one. What a resource at rest takes from its node without a
// transition does not count, so a state reached only that way, missing say,
// needs one transition that leads to it: the machine says where its states
// come from, and a typo in one cannot go unnoticed.
func (m Machine) Validate() []error {
	var problems []error

	add := func(format string, args ...any) {
		problems = append(problems, fmt.Errorf(format, args...))
	}

	if len(m.States) == 0 {
		add("it has no states")

		return problems
	}

	listed := make(map[State]bool, len(m.States))
	for _, s := range m.States {
		switch {
		case len(s) == 0:
			add("a state has no name")
		case s == Any:
			add("%q is not a state: it stands for every state", Any)
		case listed[s]:
			add("state %q is listed twice", s)
		}

		listed[s] = true
	}

	if !m.Has(m.Initial) {
		add("its initial state %q is not one of its states", m.Initial)
	}

	m.validateSorts(add)
	m.validateTransitions(add)
	m.validateReach(add)

	return problems
}

// validateSorts holds the terminal and in-flight states to being states, and
// to being one sort at most.
func (m Machine) validateSorts(add func(format string, args ...any)) {
	for _, sort := range []struct {
		name   string
		states []State
	}{
		{name: "terminal", states: m.Terminal},
		{name: "in-flight", states: m.InFlight},
	} {
		seen := make(map[State]bool, len(sort.states))

		for _, s := range sort.states {
			if !m.Has(s) {
				add("%s state %q is not one of its states", sort.name, s)
			}

			if seen[s] {
				add("%s state %q is listed twice", sort.name, s)
			}

			seen[s] = true
		}
	}

	for _, s := range m.Terminal {
		if m.IsInFlight(s) {
			add("state %q is both terminal and in flight", s)
		}
	}
}

// validateTransitions holds every transition to leading from a state to a
// state on one trigger, and to being the only one on that trigger from that
// state.
func (m Machine) validateTransitions(add func(format string, args ...any)) {
	type key struct {
		from State
		on   Trigger
	}

	declared := make(map[key]bool, len(m.Transitions))

	for _, t := range m.Transitions {
		if t.From != Any && !m.Has(t.From) {
			add("a transition on %s is from %q, which is not one of its states", t.On, t.From)
		}

		if !m.Has(t.To) {
			add("a transition on %s from %q is to %q, which is not one of its states", t.On, t.From, t.To)
		}

		if !t.On.valid() {
			add("a transition from %q is on an action or on an observation, and this one is on both or neither", t.From)
		}

		if observed := t.On.Observed; len(observed) > 0 && observed != Missing && !m.Has(observed) {
			add("a transition from %q is on observing %q, which is not one of its states", t.From, observed)
		}

		if declared[key{from: t.From, on: t.On}] {
			add("there are two transitions on %s from %q", t.On, t.From)
		}

		declared[key{from: t.From, on: t.On}] = true
	}

	for _, s := range m.InFlight {
		out := false
		for _, t := range m.leaving(s) {
			out = out || t.To != s
		}

		if !out {
			add("in-flight state %q has no transition out of it: a resource in it would wait forever", s)
		}
	}
}

// validateReach holds every state to being reached from the initial one by
// declared transitions.
func (m Machine) validateReach(add func(format string, args ...any)) {
	if !m.Has(m.Initial) {
		return
	}

	reached := map[State]bool{m.Initial: true}

	for queue := []State{m.Initial}; len(queue) > 0; queue = queue[1:] {
		for _, t := range m.leaving(queue[0]) {
			if !reached[t.To] {
				reached[t.To] = true
				queue = append(queue, t.To)
			}
		}
	}

	for _, s := range m.States {
		if !reached[s] {
			add("state %q cannot be reached from %q", s, m.Initial)
		}
	}
}
