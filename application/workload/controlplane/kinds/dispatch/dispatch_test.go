package dispatch_test

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/khanzadimahdi/testproject/application/workload/controlplane/kinds/dispatch"
	"github.com/khanzadimahdi/testproject/application/workload/controlplane/kinds/kindstest"
	"github.com/khanzadimahdi/testproject/application/workload/controlplane/kinds/waiters"
	"github.com/khanzadimahdi/testproject/domain"
	"github.com/khanzadimahdi/testproject/domain/workload/kind"
	"github.com/khanzadimahdi/testproject/domain/workload/resource"
	messagingMock "github.com/khanzadimahdi/testproject/infrastructure/messaging/mock"
	resourcesMemory "github.com/khanzadimahdi/testproject/infrastructure/repository/memory/workload/resources"
)

// fixture is a dispatcher over a fan or two kept in memory.
type fixture struct {
	resources *resourcesMemory.Repository
	producer  *messagingMock.Recorder
	waiters   *waiters.Waiters
	clock     *kindstest.Clock
	fans      *kindstest.Fans
	binding   kind.ControlPlaneBinding
	dispatch  *dispatch.Dispatcher
}

func newFixture(t *testing.T, records ...resource.Record) *fixture {
	t.Helper()

	f := &fixture{
		resources: resourcesMemory.NewRepository(),
		producer:  &messagingMock.Recorder{},
		waiters:   waiters.New(),
		clock:     kindstest.NewClock(),
		fans:      &kindstest.Fans{Node: kindstest.NodeName},
	}

	f.binding, _ = kindstest.Registry(f.fans).Lookup(kindstest.Kind)
	f.dispatch = dispatch.New(f.resources, f.producer, f.waiters, f.clock.Now, dispatch.PollEvery(time.Millisecond))

	for _, r := range records {
		_, err := f.resources.Create(context.Background(), r)
		require.NoError(t, err)
	}

	return f
}

func (f *fixture) stored(t *testing.T, uuid string) resource.Record {
	t.Helper()

	r, err := f.resources.GetOne(context.Background(), kindstest.Kind, uuid)
	require.NoError(t, err)

	return r
}

func TestDispatcher_Ask(t *testing.T) {
	t.Parallel()

	ctx := context.Background()

	t.Run("a command for a node moves the resource, is what it waits on, and is written down before it is sent", func(t *testing.T) {
		t.Parallel()

		f := newFixture(t, kindstest.AFan("fan-uuid", kindstest.Stopped, kindstest.Stopped, func(fan *kindstest.Fan) {
			fan.Status.Reason = "it was stopped"
		}))
		f.clock.Advance(time.Minute)

		asked, invalid, err := f.dispatch.Ask(ctx, f.binding, f.stored(t, "fan-uuid"), "start", json.RawMessage(" { \"speed\" : 2 } "), true)
		require.NoError(t, err)
		require.Empty(t, invalid)

		require.NotNil(t, asked.Command)
		assert.False(t, asked.Gone)

		command := *asked.Command
		assert.NotEmpty(t, command.ID)
		assert.Equal(t, kindstest.Kind, command.Kind)
		assert.Equal(t, "fan-uuid", command.UUID)
		assert.Equal(t, "start", command.Action)
		assert.Equal(t, kindstest.NodeName, command.Node)
		assert.Equal(t, 0, command.Attempt)
		assert.Equal(t, `{"speed":2}`, string(command.Payload), "a payload travels as JSON with nothing to spare")

		stored := f.stored(t, "fan-uuid")
		fan := kindstest.Typed(stored)

		assert.Equal(t, kindstest.Starting, fan.Status.State)
		assert.Equal(t, kindstest.Running, fan.Status.Expected, "what it desires is what it is expected to be")
		assert.Equal(t, f.clock.Now(), fan.Status.Since)
		assert.Empty(t, fan.Status.Reason, "why it was what it was is no longer")
		assert.Equal(t, f.clock.Now(), stored.Metadata.UpdatedAt)

		require.NotNil(t, stored.Pending)
		assert.Equal(t, "start", stored.Pending.Action)
		assert.Equal(t, []string{command.ID}, stored.Pending.IDs)
		assert.JSONEq(t, `{"speed":2}`, string(stored.Pending.Payload))
		assert.Equal(t, f.clock.Now(), stored.Pending.SentAt)
		assert.Equal(t, 1, stored.Attempts)
		assert.Equal(t, f.clock.Now(), stored.TriedAt)

		assert.Equal(t, stored.Raw, command.Resource, "a command carries the resource as it was written down")
		assert.Equal(t, stored, asked.Record)
		assert.Empty(t, f.producer.Messages(), "it is sent once it has been written down, by Send")
	})

	t.Run("asked again by the reconcile loop, it is another try", func(t *testing.T) {
		t.Parallel()

		record := kindstest.AFan("fan-uuid", kind.Failed, kindstest.Running)
		record.Attempts = 2

		f := newFixture(t, record)

		asked, _, err := f.dispatch.Ask(ctx, f.binding, f.stored(t, "fan-uuid"), "start", nil, false)
		require.NoError(t, err)

		assert.Equal(t, 2, asked.Command.Attempt)
		assert.Equal(t, 3, f.stored(t, "fan-uuid").Attempts)
	})

	t.Run("while asked by somebody, it is something new: the tries start again", func(t *testing.T) {
		t.Parallel()

		record := kindstest.AFan("fan-uuid", kind.Failed, kindstest.Running)
		record.Attempts = 2

		f := newFixture(t, record)

		asked, _, err := f.dispatch.Ask(ctx, f.binding, f.stored(t, "fan-uuid"), "start", nil, true)
		require.NoError(t, err)

		assert.Equal(t, 0, asked.Command.Attempt)
		assert.Equal(t, 1, f.stored(t, "fan-uuid").Attempts)
	})

	for name, tt := range map[string]struct {
		state   kind.State
		action  string
		payload string
		invalid domain.ValidationErrors
		err     error
	}{
		"a command its state does not allow is refused": {
			state: kindstest.Stopped, action: "stop",
			invalid: domain.ValidationErrors{"action": "invalid_state_transition"},
		},
		"and so is a payload that is not valid": {
			state: kindstest.Stopped, action: "start", payload: `{"speed": 9}`,
			invalid: domain.ValidationErrors{"speed": "invalid_value"},
		},
		"a payload that cannot be read cannot be asked with": {
			state: kindstest.Stopped, action: "start", payload: `{"speed": "fast"}`,
			err: kind.ErrInvalidPayload,
		},
		"nor can a command asked with nothing be asked with something": {
			state: kindstest.Running, action: "stop", payload: `{"now": true}`,
			err: kind.ErrInvalidPayload,
		},
		"a command the kind does not have is not asked": {
			state: kindstest.Running, action: "explode",
			err: kind.ErrUnknownAction,
		},
		"and a query is not a command": {
			state: kindstest.Running, action: "state",
			err: kind.ErrUnknownAction,
		},
		"a strategy's refusal is said field by field": {
			state: kindstest.Running, action: "rename", payload: `{"name": "taken"}`,
			invalid: domain.ValidationErrors{"name": "invalid_value"},
		},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			f := newFixture(t, kindstest.AFan("fan-uuid", tt.state, tt.state))

			_, invalid, err := f.dispatch.Ask(ctx, f.binding, f.stored(t, "fan-uuid"), tt.action, json.RawMessage(tt.payload), true)

			if tt.err != nil {
				assert.ErrorIs(t, err, tt.err)
			} else {
				require.NoError(t, err)
			}

			assert.Equal(t, tt.invalid, invalid)
			assert.Equal(t, int64(1), f.stored(t, "fan-uuid").Version, "nothing is written")
		})
	}

	t.Run("a command run in the control plane is carried out there, and its outcome written down", func(t *testing.T) {
		t.Parallel()

		record := kindstest.AFan("fan-uuid", kindstest.Running, kindstest.Running)
		record.Pending = &resource.Pending{Action: "dust", IDs: []string{"command-1"}}

		f := newFixture(t, record)

		asked, invalid, err := f.dispatch.Ask(ctx, f.binding, f.stored(t, "fan-uuid"), "rename", json.RawMessage(`{"name": "hall"}`), true)
		require.NoError(t, err)
		require.Empty(t, invalid)

		assert.Nil(t, asked.Command, "nothing is sent to its node")

		stored := f.stored(t, "fan-uuid")
		assert.Equal(t, "hall", stored.Metadata.Name)
		assert.Equal(t, 1, kindstest.Typed(stored).Status.Renames)
		assert.Equal(t, kindstest.Running, kindstest.Typed(stored).Status.State)
		assert.Equal(t, "fan-uuid", stored.Metadata.UUID)
		assert.NotNil(t, stored.Pending, "what it waits on of its node it waits on still")
		assert.Empty(t, f.producer.Messages())
	})

	t.Run("one the strategy could not carry out is an error, and nothing is written", func(t *testing.T) {
		t.Parallel()

		f := newFixture(t, kindstest.AFan("fan-uuid", kindstest.Running, kindstest.Running))
		f.fans.Failure = errors.New("the database is gone")

		_, _, err := f.dispatch.Ask(ctx, f.binding, f.stored(t, "fan-uuid"), "rename", json.RawMessage(`{"name": "hall"}`), true)

		assert.ErrorIs(t, err, f.fans.Failure)
		assert.Equal(t, int64(1), f.stored(t, "fan-uuid").Version)
	})

	t.Run("a command for a node, asked of a resource on none, has what it desires written down, and moves nothing", func(t *testing.T) {
		t.Parallel()

		f := newFixture(t, kindstest.AFan("fan-uuid", kindstest.Stopped, kindstest.Stopped, func(fan *kindstest.Fan) {
			fan.Metadata.Node = ""
		}))
		f.clock.Advance(time.Minute)

		asked, invalid, err := f.dispatch.Ask(ctx, f.binding, f.stored(t, "fan-uuid"), "start", nil, true)
		require.NoError(t, err)
		require.Empty(t, invalid)

		assert.Nil(t, asked.Command, "there is nowhere to send it")
		assert.False(t, asked.Gone)

		stored := f.stored(t, "fan-uuid")
		fan := kindstest.Typed(stored)

		assert.Equal(t, kindstest.Stopped, fan.Status.State, "it stays where it is")
		assert.Equal(t, kindstest.Running, fan.Status.Expected, "and is to be running once it is somewhere")
		assert.Nil(t, stored.Pending, "waiting on nothing")
		assert.Equal(t, f.clock.Now(), stored.Metadata.UpdatedAt)
		assert.Equal(t, stored, asked.Record)
		assert.Empty(t, f.producer.Messages())
	})

	t.Run("and one that desires nothing cannot be sent to it at all", func(t *testing.T) {
		t.Parallel()

		f := newFixture(t, kindstest.AFan("fan-uuid", kindstest.Stopped, kindstest.Stopped, func(fan *kindstest.Fan) {
			fan.Metadata.Node = ""
		}))

		d := kindstest.Descriptor()
		for i := range d.Actions {
			if d.Actions[i].Name == "start" {
				d.Actions[i].Desires = ""
			}
		}

		desiring := kind.BindControlPlane[kindstest.Spec, kindstest.Status](d, f.fans)

		_, _, err := f.dispatch.Ask(ctx, desiring, f.stored(t, "fan-uuid"), "start", nil, true)

		assert.ErrorIs(t, err, kind.ErrUnreachable)
		assert.Equal(t, int64(1), f.stored(t, "fan-uuid").Version)
	})

	t.Run("but one deleted there has nothing anywhere to delete, and goes", func(t *testing.T) {
		t.Parallel()

		f := newFixture(t, kindstest.AFan("fan-uuid", kindstest.Pending, kindstest.Running, func(fan *kindstest.Fan) {
			fan.Metadata.Node = ""
		}))

		asked, _, err := f.dispatch.Ask(ctx, f.binding, f.stored(t, "fan-uuid"), "delete", nil, true)
		require.NoError(t, err)

		assert.True(t, asked.Gone)
		assert.Nil(t, asked.Command)
		assert.Equal(t, 0, f.resources.Len(kindstest.Kind))
	})

	t.Run("a delete desires deleted, from whatever state", func(t *testing.T) {
		t.Parallel()

		f := newFixture(t, kindstest.AFan("fan-uuid", kindstest.Starting, kindstest.Running))

		asked, _, err := f.dispatch.Ask(ctx, f.binding, f.stored(t, "fan-uuid"), "delete", nil, true)
		require.NoError(t, err)

		fan := kindstest.Typed(f.stored(t, "fan-uuid"))
		assert.Equal(t, kindstest.Deleting, fan.Status.State)
		assert.Equal(t, kind.Deleted, fan.Status.Expected)
		assert.Equal(t, "delete", asked.Command.Action)
	})

	t.Run("a copy read before something else wrote the resource is not written over it", func(t *testing.T) {
		t.Parallel()

		f := newFixture(t, kindstest.AFan("fan-uuid", kindstest.Running, kindstest.Running))

		read := f.stored(t, "fan-uuid")

		_, _, err := f.dispatch.Ask(ctx, f.binding, f.stored(t, "fan-uuid"), "stop", nil, true)
		require.NoError(t, err)

		_, _, err = f.dispatch.Ask(ctx, f.binding, read, "delete", nil, true)

		assert.ErrorIs(t, err, resource.ErrConflict)
		assert.Equal(t, kindstest.Stopping, kindstest.Typed(f.stored(t, "fan-uuid")).Status.State)
	})
}

// preparingFans are fans whose starts are readied before they are sent: one
// on no node is placed on node-2, one with a blade too many is refused, and
// every one is given the speed it was asked for as its renames, which only
// the control plane writes.
type preparingFans struct {
	*kindstest.Fans

	failure error
}

var _ kind.Preparer[kindstest.Spec, kindstest.Status] = &preparingFans{}

func (p *preparingFans) Prepare(_ context.Context, r kindstest.Fan, action string, payload any) (kindstest.Fan, domain.ValidationErrors, error) {
	if p.failure != nil {
		return kindstest.Fan{}, nil, p.failure
	}

	if action != "start" {
		return r, nil, nil
	}

	if r.Spec.Blades > 5 {
		return kindstest.Fan{}, domain.ValidationErrors{"blades": "too_many"}, nil
	}

	if len(r.Metadata.Node) == 0 {
		r.Metadata.Node = "node-2"
	}

	r.Status.Renames = payload.(kindstest.StartPayload).Speed

	return r, nil, nil
}

func TestDispatcher_Ask_prepared(t *testing.T) {
	t.Parallel()

	ctx := context.Background()

	prepared := func(f *fixture, failure error) kind.ControlPlaneBinding {
		return kind.BindControlPlane[kindstest.Spec, kindstest.Status](kindstest.Descriptor(), &preparingFans{Fans: f.fans, failure: failure})
	}

	t.Run("a command for a node is readied by its kind, and carries the resource as it was readied", func(t *testing.T) {
		t.Parallel()

		f := newFixture(t, kindstest.AFan("fan-uuid", kindstest.Stopped, kindstest.Stopped, func(fan *kindstest.Fan) {
			fan.Metadata.Node = ""
		}))

		asked, invalid, err := f.dispatch.Ask(ctx, prepared(f, nil), f.stored(t, "fan-uuid"), "start", json.RawMessage(`{"speed":3}`), true)
		require.NoError(t, err)
		require.Empty(t, invalid)

		require.NotNil(t, asked.Command, "placed, it has somewhere to be sent")
		assert.Equal(t, "node-2", asked.Command.Node)

		stored := f.stored(t, "fan-uuid")
		fan := kindstest.Typed(stored)

		assert.Equal(t, "node-2", stored.Metadata.Node, "written down where it was placed")
		assert.Equal(t, 3, fan.Status.Renames, "with what the kind gave it")
		assert.Equal(t, kindstest.Starting, fan.Status.State, "and moved as its machine says")
		assert.Equal(t, stored.Raw, asked.Command.Resource)
	})

	t.Run("what the kind refuses is said, and nothing is written or sent", func(t *testing.T) {
		t.Parallel()

		f := newFixture(t, kindstest.AFan("fan-uuid", kindstest.Stopped, kindstest.Stopped, func(fan *kindstest.Fan) {
			fan.Spec.Blades = 6
		}))

		asked, invalid, err := f.dispatch.Ask(ctx, prepared(f, nil), f.stored(t, "fan-uuid"), "start", nil, true)
		require.NoError(t, err)

		assert.Equal(t, domain.ValidationErrors{"blades": "too_many"}, invalid)
		assert.Nil(t, asked.Command)
		assert.Equal(t, int64(1), f.stored(t, "fan-uuid").Version)
	})

	t.Run("and what kept it from looking is an error", func(t *testing.T) {
		t.Parallel()

		f := newFixture(t, kindstest.AFan("fan-uuid", kindstest.Stopped, kindstest.Stopped))
		failure := errors.New("the snapshots cannot be read")

		_, _, err := f.dispatch.Ask(ctx, prepared(f, failure), f.stored(t, "fan-uuid"), "start", nil, true)

		assert.ErrorIs(t, err, failure)
		assert.Equal(t, int64(1), f.stored(t, "fan-uuid").Version)
	})

	t.Run("its uuid and its kind are not the kind's to change", func(t *testing.T) {
		t.Parallel()

		f := newFixture(t, kindstest.AFan("fan-uuid", kindstest.Running, kindstest.Running))

		asked, _, err := f.dispatch.Ask(ctx, prepared(f, nil), f.stored(t, "fan-uuid"), "stop", nil, true)
		require.NoError(t, err)

		assert.Equal(t, "fan-uuid", asked.Command.UUID)
		assert.Equal(t, kindstest.Kind, asked.Command.Resource.Kind)
	})
}

func TestDispatcher_Follow(t *testing.T) {
	t.Parallel()

	ctx := context.Background()

	t.Run("what a kind asks for of a resource is asked, until a command for its node, which is returned to be sent", func(t *testing.T) {
		t.Parallel()

		f := newFixture(t, kindstest.AFan("fan-uuid", kindstest.Stopped, kindstest.Running))
		f.fans.Intents = func(kindstest.Fan) []kind.Intent {
			return []kind.Intent{
				{Action: "rename", Payload: kindstest.RenamePayload{Name: "hall"}},
				{Action: "start", Payload: kindstest.StartPayload{Speed: 2}},
				{Action: "stop"},
			}
		}

		asked, err := f.dispatch.Follow(ctx, f.binding, f.stored(t, "fan-uuid"))
		require.NoError(t, err)

		require.NotNil(t, asked.Command)
		assert.Equal(t, "start", asked.Command.Action, "the first command for its node, and nothing after it")
		assert.JSONEq(t, `{"speed":2}`, string(asked.Command.Payload))

		stored := f.stored(t, "fan-uuid")
		assert.Equal(t, "hall", stored.Metadata.Name, "what ran in the control plane before it was carried out")
		assert.Equal(t, kindstest.Starting, kindstest.Typed(stored).Status.State)
		assert.Equal(t, stored, asked.Record)
		assert.Empty(t, f.producer.Messages(), "it is the caller's to send")
	})

	t.Run("nothing to ask for leaves the resource as it is", func(t *testing.T) {
		t.Parallel()

		f := newFixture(t, kindstest.AFan("fan-uuid", kindstest.Running, kindstest.Running))

		asked, err := f.dispatch.Follow(ctx, f.binding, f.stored(t, "fan-uuid"))
		require.NoError(t, err)

		assert.Nil(t, asked.Command)
		assert.Equal(t, f.stored(t, "fan-uuid"), asked.Record)
	})

	t.Run("what its state does not allow, or is refused, is left for the reconcile loop", func(t *testing.T) {
		t.Parallel()

		f := newFixture(t, kindstest.AFan("fan-uuid", kindstest.Running, kindstest.Running))
		f.fans.Intents = func(kindstest.Fan) []kind.Intent {
			return []kind.Intent{{Action: "start"}}
		}

		asked, err := f.dispatch.Follow(ctx, f.binding, f.stored(t, "fan-uuid"))
		require.NoError(t, err)
		assert.Nil(t, asked.Command)

		f.fans.Intents = func(kindstest.Fan) []kind.Intent {
			return []kind.Intent{{Action: "rename", Payload: kindstest.RenamePayload{}}}
		}

		asked, err = f.dispatch.Follow(ctx, f.binding, f.stored(t, "fan-uuid"))
		require.NoError(t, err)
		assert.Nil(t, asked.Command)
		assert.Equal(t, int64(1), f.stored(t, "fan-uuid").Version, "nothing was written")
	})

	t.Run("what the kind could not decide is an error", func(t *testing.T) {
		t.Parallel()

		f := newFixture(t, kindstest.AFan("fan-uuid", kindstest.Stopped, kindstest.Running))
		f.fans.Failure = errors.New("the database is gone")

		_, err := f.dispatch.Follow(ctx, f.binding, f.stored(t, "fan-uuid"))

		assert.ErrorIs(t, err, f.fans.Failure)
	})
}

func TestDispatcher_Again(t *testing.T) {
	t.Parallel()

	ctx := context.Background()

	t.Run("the command a resource waits on is sent once more, as its next try", func(t *testing.T) {
		t.Parallel()

		record := kindstest.AFan("fan-uuid", kindstest.Starting, kindstest.Running)
		record.Pending = &resource.Pending{Action: "start", Payload: json.RawMessage(`{"speed":2}`), IDs: []string{"command-1"}, SentAt: kindstest.Moment}
		record.Attempts = 1

		f := newFixture(t, record)
		f.clock.Advance(10 * time.Minute)

		asked, err := f.dispatch.Again(ctx, f.stored(t, "fan-uuid"))
		require.NoError(t, err)

		command := *asked.Command
		assert.NotEqual(t, "command-1", command.ID, "a try has an ID of its own")
		assert.Equal(t, "start", command.Action)
		assert.Equal(t, 1, command.Attempt)
		assert.JSONEq(t, `{"speed":2}`, string(command.Payload))

		stored := f.stored(t, "fan-uuid")
		assert.Equal(t, []string{"command-1", command.ID}, stored.Pending.IDs, "a result for either try answers it")
		assert.Equal(t, f.clock.Now(), stored.Pending.SentAt)
		assert.Equal(t, 2, stored.Attempts)
		assert.Equal(t, kindstest.Starting, kindstest.Typed(stored).Status.State, "it is on its way already")
		assert.Equal(t, kindstest.Moment, kindstest.Typed(stored).Status.Since)
	})

	t.Run("only so many tries are remembered", func(t *testing.T) {
		t.Parallel()

		record := kindstest.AFan("fan-uuid", kindstest.Starting, kindstest.Running)
		record.Pending = &resource.Pending{Action: "start", IDs: []string{"command-1"}}

		f := newFixture(t, record)

		var last string
		for range 20 {
			asked, err := f.dispatch.Again(ctx, f.stored(t, "fan-uuid"))
			require.NoError(t, err)

			last = asked.Command.ID
		}

		ids := f.stored(t, "fan-uuid").Pending.IDs
		assert.Len(t, ids, 16)
		assert.Equal(t, last, ids[len(ids)-1])
		assert.NotContains(t, ids, "command-1")
	})

	t.Run("one that waits on nothing has nothing to send again", func(t *testing.T) {
		t.Parallel()

		f := newFixture(t, kindstest.AFan("fan-uuid", kindstest.Starting, kindstest.Running))

		_, err := f.dispatch.Again(ctx, f.stored(t, "fan-uuid"))

		assert.Error(t, err)
	})
}

func TestDispatcher_Desire(t *testing.T) {
	t.Parallel()

	record := kindstest.AFan("fan-uuid", kindstest.Starting, kindstest.Running)
	record.Attempts = 3

	f := newFixture(t, record)

	_, err := f.dispatch.Desire(context.Background(), f.stored(t, "fan-uuid"), kind.Deleted, true)
	require.NoError(t, err)

	stored := f.stored(t, "fan-uuid")
	assert.Equal(t, kind.Deleted, kindstest.Typed(stored).Status.Expected)
	assert.Equal(t, kindstest.Starting, kindstest.Typed(stored).Status.State, "nothing is asked of it yet")
	assert.Equal(t, 0, stored.Attempts)
}

// answering is a node that answers every command as soon as it is sent,
// with what answer says.
type answering struct {
	waiters *waiters.Waiters
	answer  func(command kind.ActOnResource) kind.ResourceActedOn
}

func (a *answering) Produce(_ context.Context, subject string, payload []byte) error {
	var command kind.ActOnResource
	if err := json.Unmarshal(payload, &command); err != nil {
		return err
	}

	go a.waiters.Answer(a.answer(command))

	return nil
}

func TestDispatcher_Send(t *testing.T) {
	t.Parallel()

	ctx := context.Background()

	command := kind.ActOnResource{ID: "command-1", Kind: kindstest.Kind, UUID: "fan-uuid", Action: "start", Node: kindstest.NodeName}

	t.Run("a command is sent on workloadActOnResource, to the node it is addressed to", func(t *testing.T) {
		t.Parallel()

		f := newFixture(t)

		result, err := f.dispatch.Send(ctx, command, 0)
		require.NoError(t, err)
		assert.Nil(t, result, "nothing is waited for")

		sent, err := messagingMock.Produced[kind.ActOnResource](f.producer, kind.ActOnResourceName)
		require.NoError(t, err)
		require.Len(t, sent, 1)

		assert.Equal(t, command.ID, sent[0].ID)
		assert.Equal(t, command.Kind, sent[0].Kind)
		assert.Equal(t, command.UUID, sent[0].UUID)
		assert.Equal(t, command.Action, sent[0].Action)
		assert.Equal(t, kindstest.NodeName, sent[0].Node)
	})

	t.Run("what came of it is waited for, when asked to be", func(t *testing.T) {
		t.Parallel()

		f := newFixture(t)
		node := &answering{waiters: f.waiters, answer: func(c kind.ActOnResource) kind.ResourceActedOn {
			return kind.ResourceActedOn{ID: c.ID, Kind: c.Kind, UUID: c.UUID, Action: c.Action, OK: true, Output: "started"}
		}}

		result, err := dispatch.New(f.resources, node, f.waiters, f.clock.Now).Send(ctx, command, time.Minute)
		require.NoError(t, err)

		require.NotNil(t, result)
		assert.Equal(t, "started", result.Output)
		assert.Equal(t, 0, f.waiters.Len(), "the wait is over")
	})

	t.Run("and found on the resource, when another control plane heard it", func(t *testing.T) {
		t.Parallel()

		record := kindstest.AFan("fan-uuid", kindstest.Running, kindstest.Running)
		record.Answer = &kind.ResourceActedOn{ID: "command-1", Kind: kindstest.Kind, UUID: "fan-uuid", Action: "start", OK: false, Reason: "the blades are stuck"}

		f := newFixture(t, record)

		result, err := f.dispatch.Send(ctx, command, time.Minute)
		require.NoError(t, err)

		require.NotNil(t, result)
		assert.Equal(t, "the blades are stuck", result.Reason)
	})

	for name, tt := range map[string]struct {
		action string
		ok     bool
	}{
		"a resource that is gone was deleted, which is what came of a delete": {action: "delete", ok: true},
		"and what overtook anything else":                                     {action: "start", ok: false},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			f := newFixture(t)

			asked := command
			asked.Action = tt.action

			result, err := f.dispatch.Send(ctx, asked, time.Minute)
			require.NoError(t, err)

			require.NotNil(t, result)
			assert.Equal(t, tt.ok, result.OK)
			assert.Equal(t, "command-1", result.ID)
		})
	}

	t.Run("nothing that came in time is no answer", func(t *testing.T) {
		t.Parallel()

		f := newFixture(t, kindstest.AFan("fan-uuid", kindstest.Starting, kindstest.Running))

		result, err := f.dispatch.Send(ctx, command, 20*time.Millisecond)
		require.NoError(t, err)

		assert.Nil(t, result)
		assert.Equal(t, 0, f.waiters.Len())
	})

	t.Run("a command that cannot be sent is an error", func(t *testing.T) {
		t.Parallel()

		f := newFixture(t)
		f.producer.Err = errors.New("nats is away")

		_, err := f.dispatch.Send(ctx, command, time.Minute)

		assert.ErrorIs(t, err, f.producer.Err)
		assert.Equal(t, 0, f.waiters.Len())
	})
}

func TestDispatcher_Deliver(t *testing.T) {
	t.Parallel()

	ctx := context.Background()

	t.Run("a command run in place is delivered as the resource it left", func(t *testing.T) {
		t.Parallel()

		f := newFixture(t)
		record := kindstest.AFan("fan-uuid", kindstest.Running, kindstest.Running)

		delivered, err := f.dispatch.Deliver(ctx, dispatch.Asked{Record: record}, time.Minute)
		require.NoError(t, err)

		assert.Equal(t, dispatch.Delivered{Resource: record.Raw}, delivered)
	})

	t.Run("and one that took the resource away as nothing", func(t *testing.T) {
		t.Parallel()

		f := newFixture(t)

		delivered, err := f.dispatch.Deliver(ctx, dispatch.Asked{Record: kindstest.AFan("fan-uuid", kindstest.Running, kindstest.Running), Gone: true}, 0)
		require.NoError(t, err)

		assert.Equal(t, dispatch.Delivered{Gone: true}, delivered)
	})

	t.Run("a command for a node is sent, and once answered, the resource is as the answer left it", func(t *testing.T) {
		t.Parallel()

		f := newFixture(t, kindstest.AFan("fan-uuid", kindstest.Stopped, kindstest.Stopped))

		asked, _, err := f.dispatch.Ask(ctx, f.binding, f.stored(t, "fan-uuid"), "start", nil, true)
		require.NoError(t, err)

		// the result, heard and written down by another control plane.
		go func() {
			time.Sleep(5 * time.Millisecond)

			stored := f.stored(t, "fan-uuid")
			stored.Answer = &kind.ResourceActedOn{ID: asked.Command.ID, Action: "start", OK: true}
			common, _ := stored.Common()
			common.State = kindstest.Running
			_ = stored.SetCommon(common)

			_, _ = f.resources.Update(context.Background(), stored)
		}()

		delivered, err := f.dispatch.Deliver(ctx, asked, time.Minute)
		require.NoError(t, err)

		require.NotNil(t, delivered.Result)
		assert.True(t, delivered.Result.OK)
		assert.Equal(t, asked.Command, delivered.Command)
		assert.Equal(t, kindstest.Running, kindstest.Typed(resource.Record{Raw: delivered.Resource}).Status.State)
	})
}

func TestDispatcher_Orphaned(t *testing.T) {
	t.Parallel()

	ctx := context.Background()

	t.Run("a node is asked to delete what it holds that nobody keeps a record of, once a minute at most", func(t *testing.T) {
		t.Parallel()

		f := newFixture(t)

		require.NoError(t, f.dispatch.Orphaned(ctx, kindstest.Descriptor(), kindstest.NodeName, "orphan"))
		require.NoError(t, f.dispatch.Orphaned(ctx, kindstest.Descriptor(), kindstest.NodeName, "orphan"))

		sent, err := messagingMock.Produced[kind.ActOnResource](f.producer, kind.ActOnResourceName)
		require.NoError(t, err)
		require.Len(t, sent, 1, "asked once, however often it is reported")

		assert.NotEmpty(t, sent[0].ID)
		assert.Equal(t, kindstest.Kind, sent[0].Kind)
		assert.Equal(t, "orphan", sent[0].UUID)
		assert.Equal(t, "delete", sent[0].Action)
		assert.Equal(t, kindstest.NodeName, sent[0].Node)
		assert.Equal(t, kindstest.Kind, sent[0].Resource.Kind)
		assert.Equal(t, kind.Metadata{UUID: "orphan", Node: kindstest.NodeName}, sent[0].Resource.Metadata, "carrying nothing but what it is and where")

		require.NoError(t, f.dispatch.Orphaned(ctx, kindstest.Descriptor(), "node-2", "orphan"))

		f.clock.Advance(time.Minute)
		require.NoError(t, f.dispatch.Orphaned(ctx, kindstest.Descriptor(), kindstest.NodeName, "orphan"))

		sent, err = messagingMock.Produced[kind.ActOnResource](f.producer, kind.ActOnResourceName)
		require.NoError(t, err)
		assert.Len(t, sent, 3, "another node holding it is asked too, and the first again a minute on")
		assert.Equal(t, 0, f.resources.Len(kindstest.Kind), "and nothing is written down")
	})
}
