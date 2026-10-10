package observe_test

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/khanzadimahdi/testproject/application/workload/controlplane/kinds/kindstest"
	"github.com/khanzadimahdi/testproject/application/workload/controlplane/kinds/observe"
	"github.com/khanzadimahdi/testproject/domain/workload/kind"
	"github.com/khanzadimahdi/testproject/domain/workload/resource"
)

// later is when what a test observes was observed.
var later = kindstest.Moment.Add(time.Minute)

func status(t *testing.T, s kindstest.Status) json.RawMessage {
	t.Helper()

	raw, err := json.Marshal(s)
	require.NoError(t, err)

	return raw
}

func observed(state kind.State, speed int) kindstest.Status {
	return kindstest.Status{Status: kind.Status{State: state}, Speed: speed}
}

func TestObserve(t *testing.T) {
	t.Parallel()

	starting := &resource.Pending{Action: "start", IDs: []string{"command-1"}, SentAt: kindstest.Moment}

	for name, tt := range map[string]struct {
		recorded  kind.State
		expected  kind.State
		pending   *resource.Pending
		attempts  int
		observed  kindstest.Status
		state     kind.State
		since     time.Time
		reason    string
		speed     int
		pendingIs bool
		attemptsN int
		gone      bool
		changed   bool
	}{
		"one at rest is what its node says it is": {
			recorded: kindstest.Running, expected: kindstest.Running, observed: observed(kindstest.Stopped, 0),
			state: kindstest.Stopped, since: later, speed: 0, changed: true,
		},
		"one in flight takes the arrival its machine declares": {
			recorded: kindstest.Starting, expected: kindstest.Running, pending: starting, attempts: 1, observed: observed(kindstest.Running, 2),
			state: kindstest.Running, since: later, speed: 2, attemptsN: 1, changed: true,
		},
		"and the command it waited on is answered by its arrival, while the tries it took are kept until it stays there": {
			recorded: kindstest.Stopping, expected: kindstest.Stopped, pending: &resource.Pending{Action: "stop", IDs: []string{"command-1"}}, attempts: 2, observed: observed(kindstest.Stopped, 0),
			state: kindstest.Stopped, since: later, speed: 0, attemptsN: 2, changed: true,
		},
		"a report sent before a stop does not undo it": {
			recorded: kindstest.Stopping, expected: kindstest.Stopped, pending: &resource.Pending{Action: "stop", IDs: []string{"command-1"}}, attempts: 1, observed: observed(kindstest.Running, 3),
			state: kindstest.Stopping, since: kindstest.Moment, speed: 3, pendingIs: true, attemptsN: 1, changed: true,
		},
		"a failure is believed from anywhere, with its reason": {
			recorded: kindstest.Starting, expected: kindstest.Running, pending: starting, attempts: 1, observed: kindstest.Status{Status: kind.Status{State: kind.Failed, Reason: "it overheated"}},
			state: kind.Failed, since: later, reason: "it overheated", attemptsN: 1, changed: true,
		},
		"one that is gone from its node is missing": {
			recorded: kindstest.Running, expected: kindstest.Running, observed: observed(kind.Missing, 0),
			state: kind.Missing, since: later, changed: true,
		},
		"and one on its way out that is gone is deleted": {
			recorded: kindstest.Deleting, expected: kind.Deleted, pending: &resource.Pending{Action: "delete", IDs: []string{"command-1"}}, attempts: 1, observed: observed(kind.Missing, 0),
			state: kind.Deleted, since: later, attemptsN: 1, gone: true, changed: true,
		},
		"a node that says again what it said changes nothing worth writing": {
			recorded: kindstest.Running, expected: kindstest.Running, observed: observed(kindstest.Running, 1),
			state: kindstest.Running, since: kindstest.Moment, speed: 1,
		},
		"but a kind's own field it says otherwise is a change": {
			recorded: kindstest.Running, expected: kindstest.Running, observed: observed(kindstest.Running, 3),
			state: kindstest.Running, since: kindstest.Moment, speed: 3, changed: true,
		},
		"an observation that says no state moves nothing": {
			recorded: kindstest.Starting, expected: kindstest.Running, pending: starting, attempts: 1, observed: kindstest.Status{Speed: 2},
			state: kindstest.Starting, since: kindstest.Moment, speed: 2, pendingIs: true, attemptsN: 1, changed: true,
		},
		"a state the machine does not have is not taken": {
			recorded: kindstest.Running, expected: kindstest.Running, observed: observed("exploded", 1),
			state: kindstest.Running, since: kindstest.Moment, speed: 1,
		},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			r := kindstest.AFan("fan-uuid", tt.recorded, tt.expected, func(f *kindstest.Fan) {
				f.Status.Renames = 2
			})
			r.Pending = tt.pending
			r.Attempts = tt.attempts

			change, err := observe.Observe(kindstest.Descriptor(), &r, status(t, tt.observed), later)
			require.NoError(t, err)

			fan := kindstest.Typed(r)

			assert.Equal(t, tt.state, fan.Status.State, "state")
			assert.Equal(t, tt.expected, fan.Status.Expected, "what is expected is never a node's to say")
			assert.Equal(t, tt.since, fan.Status.Since, "since")
			assert.Equal(t, tt.reason, fan.Status.Reason, "reason")
			assert.Equal(t, later, fan.Status.ObservedAt, "observed at")
			assert.Equal(t, tt.speed, fan.Status.Speed, "a kind's own field is what was observed")
			assert.Equal(t, 2, fan.Status.Renames, "and one only the control plane knows is kept")
			assert.Equal(t, tt.pendingIs, r.Pending != nil, "pending")
			assert.Equal(t, tt.attemptsN, r.Attempts, "attempts")
			assert.Equal(t, observe.Change{Gone: tt.gone, Changed: tt.changed}, change)
		})
	}

	t.Run("a status that is not one cannot be observed", func(t *testing.T) {
		t.Parallel()

		r := kindstest.AFan("fan-uuid", kindstest.Running, kindstest.Running)

		_, err := observe.Observe(kindstest.Descriptor(), &r, json.RawMessage(`["running"]`), later)

		assert.Error(t, err)
	})
}

// TestObserve_answered holds a resource waiting for the answer to its command
// (kind.Machine.Answered) to that answer: what its node is seen doing in the
// meantime may be from before the command reached it, when the resource does
// the same before the command and after it.
func TestObserve_answered(t *testing.T) {
	t.Parallel()

	d := kindstest.Descriptor()
	d.Machine.Answered = []kind.State{kindstest.Starting}

	starting := func() resource.Record {
		r := kindstest.AFan("fan-uuid", kindstest.Starting, kindstest.Running)
		r.Pending = &resource.Pending{Action: "start", IDs: []string{"command-1"}, SentAt: kindstest.Moment}

		return r
	}

	for name, seen := range map[string]kindstest.Status{
		"seen where its command takes it, it stays where it is": observed(kindstest.Running, 2),
		"and so it does seen failed":                            {Status: kind.Status{State: kind.Failed, Reason: "it overheated"}, Speed: 2},
		"or gone":                                               observed(kind.Missing, 2),
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			r := starting()

			change, err := observe.Observe(d, &r, status(t, seen), later)
			require.NoError(t, err)

			fan := kindstest.Typed(r)

			assert.Equal(t, kindstest.Starting, fan.Status.State)
			assert.Equal(t, kindstest.Moment, fan.Status.Since)
			assert.Empty(t, fan.Status.Reason)
			assert.NotNil(t, r.Pending, "it waits for its answer still")
			assert.Equal(t, 2, fan.Status.Speed, "what else its node says is taken")
			assert.Equal(t, later, fan.Status.ObservedAt)
			assert.True(t, change.Changed)
		})
	}

	t.Run("the answer to its command takes it where it says", func(t *testing.T) {
		t.Parallel()

		r := starting()

		_, err := observe.Answer(d, &r, kind.ResourceActedOn{ID: "command-1", Action: "start", OK: true, Status: status(t, observed(kindstest.Running, 2))}, later)
		require.NoError(t, err)

		assert.Equal(t, kindstest.Running, kindstest.Typed(r).Status.State)
		assert.Nil(t, r.Pending)
	})

	t.Run("and one that failed fails it", func(t *testing.T) {
		t.Parallel()

		r := starting()

		_, err := observe.Answer(d, &r, kind.ResourceActedOn{ID: "command-1", Action: "start", Reason: "the blades are stuck"}, later)
		require.NoError(t, err)

		fan := kindstest.Typed(r)
		assert.Equal(t, kind.Failed, fan.Status.State)
		assert.Equal(t, "the blades are stuck", fan.Status.Reason)
	})
}

func TestAnswer(t *testing.T) {
	t.Parallel()

	for name, tt := range map[string]struct {
		recorded  kind.State
		expected  kind.State
		pending   *resource.Pending
		result    kind.ResourceActedOn
		state     kind.State
		reason    string
		pendingIs bool
		gone      bool
	}{
		"what a command left the resource as is taken": {
			recorded: kindstest.Starting, expected: kindstest.Running,
			pending: &resource.Pending{Action: "start", IDs: []string{"command-1"}},
			result:  kind.ResourceActedOn{ID: "command-1", Action: "start", OK: true, Status: json.RawMessage(`{"state":"running","speed":2}`)},
			state:   kindstest.Running,
		},
		"a try sent before the latest answers it as well": {
			recorded: kindstest.Starting, expected: kindstest.Running,
			pending: &resource.Pending{Action: "start", IDs: []string{"command-1", "command-2"}},
			result:  kind.ResourceActedOn{ID: "command-1", Action: "start", OK: true, Status: json.RawMessage(`{"state":"running"}`)},
			state:   kindstest.Running,
		},
		"one that says nothing leaves it waiting on its node to say where it got to": {
			recorded: kindstest.Starting, expected: kindstest.Running,
			pending:   &resource.Pending{Action: "create", IDs: []string{"command-1"}},
			result:    kind.ResourceActedOn{ID: "command-1", Action: "create", OK: true},
			state:     kindstest.Starting,
			pendingIs: true,
		},
		"one that deleted it leaves it gone, whatever it said": {
			recorded: kindstest.Deleting, expected: kind.Deleted,
			pending: &resource.Pending{Action: "delete", IDs: []string{"command-1"}},
			result:  kind.ResourceActedOn{ID: "command-1", Action: "delete", OK: true},
			state:   kindstest.Deleting,
			gone:    true,
		},
		"one that failed for good fails it, with its reason": {
			recorded: kindstest.Starting, expected: kindstest.Running,
			pending: &resource.Pending{Action: "start", IDs: []string{"command-1"}},
			result:  kind.ResourceActedOn{ID: "command-1", Action: "start", OK: false, Reason: "the blades are stuck"},
			state:   kind.Failed,
			reason:  "the blades are stuck",
		},
		"or with what failed when it said nothing": {
			recorded: kindstest.Stopping, expected: kindstest.Stopped,
			pending: &resource.Pending{Action: "stop", IDs: []string{"command-1"}},
			result:  kind.ResourceActedOn{ID: "command-1", Action: "stop", OK: false},
			state:   kind.Failed,
			reason:  "the stop failed",
		},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			r := kindstest.AFan("fan-uuid", tt.recorded, tt.expected)
			r.Pending = tt.pending

			change, err := observe.Answer(kindstest.Descriptor(), &r, tt.result, later)
			require.NoError(t, err)

			fan := kindstest.Typed(r)

			assert.Equal(t, tt.gone, change.Gone, "gone")
			assert.True(t, change.Changed)
			assert.Equal(t, tt.state, fan.Status.State, "state")
			assert.Equal(t, tt.expected, fan.Status.Expected, "what is expected stays")
			assert.Equal(t, tt.reason, fan.Status.Reason, "reason")
			assert.Equal(t, tt.pendingIs, r.Pending != nil, "pending")

			require.NotNil(t, r.Answer, "the answer is kept, for whoever waits for it elsewhere")
			assert.Equal(t, tt.result.ID, r.Answer.ID)
			assert.Nil(t, r.Answer.Status, "its status is the resource's already")
		})
	}

	t.Run("a failed command that was to make the resource nothing fails only itself", func(t *testing.T) {
		t.Parallel()

		d := kindstest.Descriptor()
		d.Actions = append(d.Actions, kind.Action{Name: "dust", Runs: kind.OnNode, Mode: kind.ModeCommand, Permission: "manage", Payload: kind.NoPayload})

		r := kindstest.AFan("fan-uuid", kindstest.Running, kindstest.Running)
		r.Pending = &resource.Pending{Action: "dust", IDs: []string{"command-1"}}

		_, err := observe.Answer(d, &r, kind.ResourceActedOn{ID: "command-1", Action: "dust", OK: false, Reason: "no duster", Status: json.RawMessage(`{"speed":2}`)}, later)
		require.NoError(t, err)

		fan := kindstest.Typed(r)

		assert.Equal(t, kindstest.Running, fan.Status.State, "it is running still")
		assert.Empty(t, fan.Status.Reason)
		assert.Equal(t, 2, fan.Status.Speed, "what the strategy said it left is taken")
		assert.Nil(t, r.Pending, "the command is answered")
		assert.Equal(t, "no duster", r.Answer.Reason)
	})

	t.Run("one its node refused as asked did nothing: it is what its node says, and expected to stay so", func(t *testing.T) {
		t.Parallel()

		r := kindstest.AFan("fan-uuid", kindstest.Deleting, kind.Deleted)
		r.Pending = &resource.Pending{Action: "delete", IDs: []string{"command-1"}}

		change, err := observe.Answer(kindstest.Descriptor(), &r, kind.ResourceActedOn{
			ID:      "command-1",
			Action:  "delete",
			OK:      false,
			Refused: true,
			Reason:  "refused: it is still plugged in",
			Status:  json.RawMessage(`{"state":"running","speed":2}`),
		}, later)
		require.NoError(t, err)

		fan := kindstest.Typed(r)

		assert.False(t, change.Gone)
		assert.True(t, change.Changed)
		assert.Equal(t, kindstest.Running, fan.Status.State)
		assert.Equal(t, kindstest.Running, fan.Status.Expected, "not to be deleted once it can be, behind the back of whoever was refused")
		assert.Empty(t, fan.Status.Reason, "it did not fail")
		assert.Equal(t, later, fan.Status.Since)
		assert.Equal(t, 2, fan.Status.Speed)
		assert.Nil(t, r.Pending)
		assert.Equal(t, "refused: it is still plugged in", r.Answer.Reason, "why is the answer's to say")
	})

	t.Run("and one whose node said nothing of it is left as it was, until its node does", func(t *testing.T) {
		t.Parallel()

		r := kindstest.AFan("fan-uuid", kindstest.Starting, kindstest.Running)
		r.Pending = &resource.Pending{Action: "start", IDs: []string{"command-1"}}

		_, err := observe.Answer(kindstest.Descriptor(), &r, kind.ResourceActedOn{ID: "command-1", Action: "start", Refused: true, Reason: "refused"}, later)
		require.NoError(t, err)

		fan := kindstest.Typed(r)

		assert.Equal(t, kindstest.Starting, fan.Status.State)
		assert.Equal(t, kindstest.Running, fan.Status.Expected)
		assert.Nil(t, r.Pending)
	})

	for name, pending := range map[string]*resource.Pending{
		"a result for a command sent before something else was asked is too late": {Action: "stop", IDs: []string{"command-2"}},
		"and so is one heard already": nil,
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			r := kindstest.AFan("fan-uuid", kindstest.Stopping, kindstest.Stopped)
			r.Pending = pending
			before := r.Clone()

			change, err := observe.Answer(kindstest.Descriptor(), &r, kind.ResourceActedOn{ID: "command-1", Action: "start", OK: true, Status: json.RawMessage(`{"state":"running"}`)}, later)

			assert.ErrorIs(t, err, observe.ErrNotWaitedOn)
			assert.Equal(t, observe.Change{}, change)
			assert.Equal(t, before, r, "nothing is changed")
		})
	}
}

func TestFail(t *testing.T) {
	t.Parallel()

	t.Run("a resource is failed for a reason, and waits on nothing any more", func(t *testing.T) {
		t.Parallel()

		r := kindstest.AFan("fan-uuid", kindstest.Starting, kindstest.Running)
		r.Pending = &resource.Pending{Action: "start", IDs: []string{"command-1"}}

		require.NoError(t, observe.Fail(&r, "node_lost", later))

		fan := kindstest.Typed(r)

		assert.Equal(t, kind.Failed, fan.Status.State)
		assert.Equal(t, "node_lost", fan.Status.Reason)
		assert.Equal(t, later, fan.Status.Since)
		assert.Equal(t, kindstest.Running, fan.Status.Expected, "what is expected of it stays, so it is brought back")
		assert.Nil(t, r.Pending)
	})

	t.Run("one failed already keeps when it failed", func(t *testing.T) {
		t.Parallel()

		r := kindstest.AFan("fan-uuid", kind.Failed, kindstest.Running)

		require.NoError(t, observe.Fail(&r, "node_lost", later))

		assert.Equal(t, kindstest.Moment, kindstest.Typed(r).Status.Since)
		assert.Equal(t, "node_lost", kindstest.Typed(r).Status.Reason)
	})
}
