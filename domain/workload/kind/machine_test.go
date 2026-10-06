package kind

import (
	"fmt"
	"slices"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestMachine_Next(t *testing.T) {
	t.Parallel()

	m := boxMachine()

	for name, tt := range map[string]struct {
		from  State
		on    Trigger
		to    State
		taken bool
	}{
		"a stopped box is started":                     {from: boxStopped, on: OnAction("start"), to: boxStarting, taken: true},
		"so is a failed one":                           {from: Failed, on: OnAction("start"), to: boxStarting, taken: true},
		"a running one is stopped":                     {from: boxRunning, on: OnAction("stop"), to: boxStopping, taken: true},
		"anything is deleted":                          {from: boxStarting, on: OnAction("delete"), to: boxDeleting, taken: true},
		"an action with no transition moves nothing":   {from: boxRunning, on: OnAction("resize"), to: boxRunning},
		"nor does one the kind does not have":          {from: boxRunning, on: OnAction("explode"), to: boxRunning},
		"a starting box that came up is running":       {from: boxStarting, on: OnObserved(boxRunning), to: boxRunning, taken: true},
		"a stopping one that stopped is stopped":       {from: boxStopping, on: OnObserved(boxStopped), to: boxStopped, taken: true},
		"one on its way out that is gone is deleted":   {from: boxDeleting, on: OnObserved(Missing), to: Deleted, taken: true},
		"a failure is believed from anywhere":          {from: boxStopping, on: OnObserved(Failed), to: Failed, taken: true},
		"a running one that is gone is missing":        {from: boxRunning, on: OnObserved(Missing), to: Missing, taken: true},
		"one at rest is what its node says it is":      {from: boxRunning, on: OnObserved(boxStopped), to: boxStopped, taken: true},
		"even when it ended":                           {from: boxStopped, on: OnObserved(boxRunning), to: boxRunning, taken: true},
		"one in flight is what it was":                 {from: boxStopping, on: OnObserved(boxStopping), to: boxStopping, taken: true},
		"a report from before a stop does not undo it": {from: boxStopping, on: OnObserved(boxRunning), to: boxStopping},
		"nor does one from before a delete":            {from: boxDeleting, on: OnObserved(boxRunning), to: boxDeleting},
		"what the machine does not have is not taken":  {from: boxStopped, on: OnObserved("exploded"), to: boxStopped},
		"a stopped one that is gone is missing too":    {from: boxStopped, on: OnObserved(Missing), to: Missing, taken: true},
		"nothing leaves deleted, by an action":         {from: Deleted, on: OnAction("delete"), to: Deleted},
		"nor by an observation":                        {from: Deleted, on: OnObserved(Failed), to: Deleted},
		"nothing moves what the machine does not have": {from: "exploded", on: OnObserved(boxRunning), to: "exploded"},
		"a trigger that is neither moves nothing":      {from: boxRunning, on: Trigger{}, to: boxRunning},
		"nor one that is both":                         {from: boxRunning, on: Trigger{Action: "stop", Observed: boxStopped}, to: boxRunning},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			to, taken := m.Next(tt.from, tt.on)

			assert.Equal(t, tt.to, to, "to")
			assert.Equal(t, tt.taken, taken, "taken")
		})
	}

	t.Run("an answered state is one in flight that waits for its command's answer", func(t *testing.T) {
		t.Parallel()

		m := boxMachine()
		m.Answered = []State{boxStarting}

		assert.True(t, m.IsAnswered(boxStarting))
		assert.False(t, m.IsAnswered(boxStopping), "in flight, and taking its arrivals from anybody")
		assert.False(t, m.IsAnswered(boxRunning))
		assert.Empty(t, m.Validate())
	})

	t.Run("a transition from the state itself goes before one from any", func(t *testing.T) {
		t.Parallel()

		m := boxMachine()
		m.Transitions = append(m.Transitions, Transition{From: boxDeleting, On: OnObserved(Failed), To: boxDeleting})

		to, taken := m.Next(boxDeleting, OnObserved(Failed))

		assert.True(t, taken)
		assert.Equal(t, boxDeleting, to)
	})

	t.Run("a kind without missing takes it only where a transition says", func(t *testing.T) {
		t.Parallel()

		m := Machine{
			Initial:     boxRunning,
			States:      []State{boxRunning, boxDeleting, Deleted},
			Transitions: []Transition{{From: boxDeleting, On: OnObserved(Missing), To: Deleted}},
			InFlight:    []State{boxDeleting},
		}

		to, taken := m.Next(boxRunning, OnObserved(Missing))
		assert.False(t, taken)
		assert.Equal(t, boxRunning, to)

		to, taken = m.Next(boxDeleting, OnObserved(Missing))
		assert.True(t, taken)
		assert.Equal(t, Deleted, to)
	})
}

func TestMachine_sorts(t *testing.T) {
	t.Parallel()

	m := boxMachine()

	for state, want := range map[State]struct {
		has      bool
		terminal bool
		inFlight bool
	}{
		boxPending:  {has: true, inFlight: true},
		boxStarting: {has: true, inFlight: true},
		boxRunning:  {has: true},
		boxStopping: {has: true, inFlight: true},
		boxStopped:  {has: true, terminal: true},
		Failed:      {has: true, terminal: true},
		Missing:     {has: true},
		boxDeleting: {has: true, inFlight: true},
		Deleted:     {has: true, terminal: true},
		"exploded":  {},
		Any:         {},
	} {
		t.Run(state.String(), func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, want.has, m.Has(state), "has")
			assert.Equal(t, want.terminal, m.IsTerminal(state), "terminal")
			assert.Equal(t, want.inFlight, m.IsInFlight(state), "in flight")
		})
	}
}

func TestMachine_Validate(t *testing.T) {
	t.Parallel()

	t.Run("a machine that holds together has nothing wrong with it", func(t *testing.T) {
		t.Parallel()

		assert.Empty(t, boxMachine().Validate())
	})

	for name, tt := range map[string]struct {
		breaking func(m *Machine)
		want     string
	}{
		"one with no states": {
			breaking: func(m *Machine) { *m = Machine{} },
			want:     "it has no states",
		},
		"a state with no name": {
			breaking: func(m *Machine) { m.States = append(m.States, "") },
			want:     "a state has no name",
		},
		"any, as a state": {
			breaking: func(m *Machine) { m.States = append(m.States, Any) },
			want:     `"*" is not a state`,
		},
		"a state listed twice": {
			breaking: func(m *Machine) { m.States = append(m.States, boxRunning) },
			want:     `state "running" is listed twice`,
		},
		"an initial state it does not have": {
			breaking: func(m *Machine) { m.Initial = "exploded" },
			want:     `its initial state "exploded" is not one of its states`,
		},
		"a terminal state it does not have": {
			breaking: func(m *Machine) { m.Terminal = append(m.Terminal, "exploded") },
			want:     `terminal state "exploded" is not one of its states`,
		},
		"an in-flight state listed twice": {
			breaking: func(m *Machine) { m.InFlight = append(m.InFlight, boxStarting) },
			want:     `in-flight state "starting" is listed twice`,
		},
		"a state of both sorts": {
			breaking: func(m *Machine) { m.Terminal = append(m.Terminal, boxStarting) },
			want:     `state "starting" is both terminal and in flight`,
		},
		"an answered state it does not have": {
			breaking: func(m *Machine) { m.Answered = append(m.Answered, "exploded") },
			want:     `answered state "exploded" is not one of its states`,
		},
		"an answered state listed twice": {
			breaking: func(m *Machine) { m.Answered = append(m.Answered, boxStarting, boxStarting) },
			want:     `answered state "starting" is listed twice`,
		},
		"an answered state at rest": {
			breaking: func(m *Machine) { m.Answered = append(m.Answered, boxRunning) },
			want:     `answered state "running" is not in flight`,
		},
		"a transition from nowhere": {
			breaking: func(m *Machine) {
				m.Transitions = append(m.Transitions, Transition{From: "exploded", On: OnAction("start"), To: boxStarting})
			},
			want: `a transition on action start is from "exploded", which is not one of its states`,
		},
		"a transition to nowhere": {
			breaking: func(m *Machine) {
				m.Transitions = append(m.Transitions, Transition{From: boxRunning, On: OnAction("explode"), To: "exploded"})
			},
			want: `a transition on action explode from "running" is to "exploded", which is not one of its states`,
		},
		"a transition on neither": {
			breaking: func(m *Machine) {
				m.Transitions = append(m.Transitions, Transition{From: boxRunning, To: boxStopped})
			},
			want: `a transition from "running" is on an action or on an observation`,
		},
		"a transition on observing what it does not have": {
			breaking: func(m *Machine) {
				m.Transitions = append(m.Transitions, Transition{From: boxStarting, On: OnObserved("exploded"), To: Failed})
			},
			want: `a transition from "starting" is on observing "exploded", which is not one of its states`,
		},
		"two transitions on one trigger from one state": {
			breaking: func(m *Machine) {
				m.Transitions = append(m.Transitions, Transition{From: boxRunning, On: OnAction("stop"), To: boxStopped})
			},
			want: `there are two transitions on action stop from "running"`,
		},
		"an in-flight state with no way out": {
			breaking: func(m *Machine) {
				m.Transitions = slices.DeleteFunc(m.Transitions, func(t Transition) bool { return t.From == boxStopping || t.From == Any })
			},
			want: `in-flight state "stopping" has no transition out of it`,
		},
		"a state nothing reaches": {
			breaking: func(m *Machine) {
				m.Transitions = slices.DeleteFunc(m.Transitions, func(t Transition) bool { return t.To == Missing })
			},
			want: `state "missing" cannot be reached from "pending"`,
		},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			m := boxMachine()
			tt.breaking(&m)

			assert.Contains(t, messages(m.Validate()), tt.want)
		})
	}

	t.Run("a transition back to where it was is no way out", func(t *testing.T) {
		t.Parallel()

		m := boxMachine()
		m.Transitions = slices.DeleteFunc(m.Transitions, func(t Transition) bool { return t.From == boxDeleting || t.From == Any })
		m.Transitions = append(m.Transitions, Transition{From: boxDeleting, On: OnObserved(boxRunning), To: boxDeleting})

		assert.Contains(t, messages(m.Validate()), `in-flight state "deleting" has no transition out of it`)
	})
}

func TestTrigger_String(t *testing.T) {
	t.Parallel()

	assert.Equal(t, "action start", OnAction("start").String())
	assert.Equal(t, "observing missing", OnObserved(Missing).String())
}

// messages are what problems say, each a whole sentence of its own, so a
// test can look for one among them.
func messages(problems []error) string {
	var said string
	for _, problem := range problems {
		said += fmt.Sprintln(problem)
	}

	return said
}
