package actOnResource_test

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/khanzadimahdi/testproject/application/workload/controlplane/kinds/actOnResource"
	"github.com/khanzadimahdi/testproject/application/workload/controlplane/kinds/dispatch"
	"github.com/khanzadimahdi/testproject/application/workload/controlplane/kinds/kindstest"
	"github.com/khanzadimahdi/testproject/application/workload/controlplane/kinds/waiters"
	"github.com/khanzadimahdi/testproject/domain"
	"github.com/khanzadimahdi/testproject/domain/workload/kind"
	"github.com/khanzadimahdi/testproject/domain/workload/noderequest"
	"github.com/khanzadimahdi/testproject/domain/workload/resource"
	messagingMock "github.com/khanzadimahdi/testproject/infrastructure/messaging/mock"
	resourcesMemory "github.com/khanzadimahdi/testproject/infrastructure/repository/memory/workload/resources"
)

type fixture struct {
	memory   *resourcesMemory.Repository
	racing   *kindstest.Racing
	producer *messagingMock.Recorder
	waiters  *waiters.Waiters
	fans     *kindstest.Fans
	useCase  *actOnResource.UseCase
}

func newFixture(t *testing.T, records ...resource.Record) *fixture {
	t.Helper()

	f := &fixture{
		memory:   resourcesMemory.NewRepository(),
		producer: &messagingMock.Recorder{},
		waiters:  waiters.New(),
		fans:     &kindstest.Fans{Node: kindstest.NodeName},
	}

	f.racing = &kindstest.Racing{Repository: f.memory}

	for _, r := range records {
		_, err := f.memory.Create(context.Background(), r)
		require.NoError(t, err)
	}

	dispatcher := dispatch.New(f.racing, f.producer, f.waiters, kindstest.NewClock().Now, dispatch.PollEvery(time.Millisecond))
	f.useCase = actOnResource.NewUseCase(registry(t, f.fans), f.racing, dispatcher, slog.New(slog.DiscardHandler))

	return f
}

// descriptor is the fan's, with what these tests ask of it beside: a nudge,
// a command for its node that desires nothing, and a start and a stop that
// wait for a fan in flight to get where it is going.
func descriptor() kind.Descriptor {
	d := kindstest.Descriptor()

	for i := range d.Actions {
		if d.Actions[i].Name == "start" || d.Actions[i].Name == "stop" {
			d.Actions[i].Waits = true
		}
	}

	d.Actions = append(d.Actions, kind.Action{
		Name:       "nudge",
		Runs:       kind.OnNode,
		Mode:       kind.ModeCommand,
		AllowedIn:  []kind.State{kindstest.Running},
		Permission: "manage",
		Payload:    kind.NoPayload,
	})

	return d
}

// registry is the fans as these tests register them, through strategy.
func registry(t *testing.T, strategy kind.ControlPlane[kindstest.Spec, kindstest.Status]) *kind.Registry[kind.ControlPlaneBinding] {
	t.Helper()

	registry := kind.NewRegistry[kind.ControlPlaneBinding]()
	require.NoError(t, registry.Register(kind.BindControlPlane[kindstest.Spec, kindstest.Status](descriptor(), strategy)))

	return registry
}

func (f *fixture) stored(t *testing.T, uuid string) kindstest.Fan {
	t.Helper()

	r, err := f.memory.GetOne(context.Background(), kindstest.Kind, uuid)
	require.NoError(t, err)

	return kindstest.Typed(r)
}

func TestUseCase_Execute(t *testing.T) {
	t.Parallel()

	ctx := context.Background()

	t.Run("a command is asked of a resource and sent to its node", func(t *testing.T) {
		t.Parallel()

		f := newFixture(t, kindstest.AFan("fan-uuid", kindstest.Stopped, kindstest.Stopped))

		response, err := f.useCase.Execute(ctx, &actOnResource.Request{
			Kind:      kindstest.Kind,
			OwnerUUID: kindstest.OwnerUUID,
			UUID:      "fan-uuid",
			Action:    "start",
			Payload:   json.RawMessage(`{"speed": 3}`),
		})
		require.NoError(t, err)

		require.Empty(t, response.ValidationErrors)
		require.NotNil(t, response.Command)
		assert.Equal(t, "start", response.Command.Action)
		assert.Nil(t, response.Result)
		assert.False(t, response.Gone)
		assert.Equal(t, kindstest.Starting, kindstest.Typed(resource.Record{Raw: response.Resource}).Status.State)

		sent, err := messagingMock.Produced[kind.Command](f.producer, kind.CommandName)
		require.NoError(t, err)
		require.Len(t, sent, 1)
		assert.Equal(t, response.Command.ID, sent[0].ID)
		assert.JSONEq(t, `{"speed":3}`, string(sent[0].Payload))

		fan := f.stored(t, "fan-uuid")
		assert.Equal(t, kindstest.Starting, fan.Status.State)
		assert.Equal(t, kindstest.Running, fan.Status.Expected)
	})

	t.Run("what came of it is waited for, when asked to be", func(t *testing.T) {
		t.Parallel()

		f := newFixture(t, kindstest.AFan("fan-uuid", kindstest.Running, kindstest.Running))

		go func() {
			for f.waiters.Len() == 0 {
				time.Sleep(time.Millisecond)
			}

			sent, _ := messagingMock.Produced[kind.Command](f.producer, kind.CommandName)
			for len(sent) == 0 {
				time.Sleep(time.Millisecond)
				sent, _ = messagingMock.Produced[kind.Command](f.producer, kind.CommandName)
			}

			f.waiters.Answer(kind.Result{ID: sent[0].ID, Kind: kindstest.Kind, UUID: "fan-uuid", Action: "stop", OK: true, Output: "stopped"})
		}()

		response, err := f.useCase.Execute(ctx, &actOnResource.Request{Kind: kindstest.Kind, UUID: "fan-uuid", Action: "stop", Wait: time.Minute})
		require.NoError(t, err)

		require.NotNil(t, response.Result)
		assert.Equal(t, "stopped", response.Result.Output)
	})

	t.Run("a command run in the control plane is carried out at once", func(t *testing.T) {
		t.Parallel()

		f := newFixture(t, kindstest.AFan("fan-uuid", kindstest.Running, kindstest.Running))

		response, err := f.useCase.Execute(ctx, &actOnResource.Request{Kind: kindstest.Kind, UUID: "fan-uuid", Action: "rename", Payload: json.RawMessage(`{"name": "hall"}`), Wait: time.Minute})
		require.NoError(t, err)

		assert.Nil(t, response.Command)
		assert.Nil(t, response.Result)
		assert.Equal(t, "hall", response.Resource.Metadata.Name)
		assert.Equal(t, "hall", f.stored(t, "fan-uuid").Metadata.Name)
		assert.Empty(t, f.producer.Messages())
	})

	t.Run("a delete with nothing anywhere to delete takes the resource away", func(t *testing.T) {
		t.Parallel()

		f := newFixture(t, kindstest.AFan("fan-uuid", kindstest.Pending, kindstest.Running, func(fan *kindstest.Fan) {
			fan.Metadata.Node = ""
		}))

		response, err := f.useCase.Execute(ctx, &actOnResource.Request{Kind: kindstest.Kind, UUID: "fan-uuid", Action: "delete"})
		require.NoError(t, err)

		assert.True(t, response.Gone)
		assert.Equal(t, kind.Raw{}, response.Resource)
		assert.Equal(t, 0, f.memory.Len(kindstest.Kind))
	})

	t.Run("one written by something else while it was asked is read again and asked again", func(t *testing.T) {
		t.Parallel()

		f := newFixture(t, kindstest.AFan("fan-uuid", kindstest.Running, kindstest.Running))

		// a heartbeat writes it down between it being read and asked.
		f.racing.Cross(kindstest.Rewrite(f.memory, func(r *resource.Record) {
			common, _ := r.Common()
			common.ObservedAt = kindstest.Moment
			_ = r.SetCommon(common)
		}))

		response, err := f.useCase.Execute(ctx, &actOnResource.Request{Kind: kindstest.Kind, UUID: "fan-uuid", Action: "stop"})
		require.NoError(t, err)

		require.NotNil(t, response.Command)

		fan := f.stored(t, "fan-uuid")
		assert.Equal(t, kindstest.Stopping, fan.Status.State)
		assert.Equal(t, kindstest.Moment, fan.Status.ObservedAt, "what the heartbeat wrote is kept")
	})

	t.Run("and one whose state moved in the meantime is asked on what it moved to", func(t *testing.T) {
		t.Parallel()

		f := newFixture(t, kindstest.AFan("fan-uuid", kindstest.Running, kindstest.Running))

		// the node said it stopped between it being read and asked.
		f.racing.Cross(kindstest.Rewrite(f.memory, func(r *resource.Record) {
			common, _ := r.Common()
			common.State = kindstest.Stopped
			_ = r.SetCommon(common)
		}))

		response, err := f.useCase.Execute(ctx, &actOnResource.Request{Kind: kindstest.Kind, UUID: "fan-uuid", Action: "stop"})
		require.NoError(t, err)

		assert.Equal(t, domain.ValidationErrors{"action": "invalid_state_transition"}, response.ValidationErrors, "a stopped fan cannot be stopped")
		assert.Empty(t, f.producer.Messages())
	})

	for name, tt := range map[string]struct {
		request  actOnResource.Request
		invalid  domain.ValidationErrors
		node     *noderequest.Error
		err      error
		unplaced bool
	}{
		"a command its state does not allow is refused": {
			request: actOnResource.Request{Action: "start"},
			invalid: domain.ValidationErrors{"action": "invalid_state_transition"},
		},
		"and so is a payload that is not valid": {
			request: actOnResource.Request{Action: "rename", Payload: json.RawMessage(`{}`)},
			invalid: domain.ValidationErrors{"name": "required_field"},
		},
		"a payload that cannot be read is refused as such": {
			request: actOnResource.Request{Action: "rename", Payload: json.RawMessage(`{"name": 7}`)},
			invalid: domain.ValidationErrors{"payload": "invalid_value"},
		},
		"a command for a node that desires nothing, of one on none, cannot be sent": {
			request:  actOnResource.Request{Action: "nudge"},
			unplaced: true,
			node:     &noderequest.Error{Code: noderequest.CodeNotRunning},
		},
		"somebody else's is not there": {
			request: actOnResource.Request{Action: "stop", OwnerUUID: "somebody-else"},
			err:     domain.ErrNotExists,
		},
		"nor is one that is not": {
			request: actOnResource.Request{Action: "stop", UUID: "another-uuid"},
			err:     domain.ErrNotExists,
		},
		"an internal command is nobody's to ask for": {
			request: actOnResource.Request{Action: "create"},
			err:     kind.ErrUnknownAction,
		},
		"a query is not a command": {
			request: actOnResource.Request{Action: "logs"},
			err:     kind.ErrUnknownAction,
		},
		"nor is what the kind does not have": {
			request: actOnResource.Request{Action: "explode"},
			err:     kind.ErrUnknownAction,
		},
		"a kind not run here is not asked": {
			request: actOnResource.Request{Kind: "kettle", Action: "stop"},
			err:     kind.ErrUnknownKind,
		},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			f := newFixture(t, kindstest.AFan("fan-uuid", kindstest.Running, kindstest.Running, func(fan *kindstest.Fan) {
				if tt.unplaced {
					fan.Metadata.Node = ""
				}
			}))

			request := tt.request
			if len(request.Kind) == 0 {
				request.Kind = kindstest.Kind
			}

			if len(request.UUID) == 0 {
				request.UUID = "fan-uuid"
			}

			response, err := f.useCase.Execute(ctx, &request)

			if tt.err != nil {
				assert.ErrorIs(t, err, tt.err)

				return
			}

			require.NoError(t, err)
			assert.Equal(t, tt.invalid, response.ValidationErrors)

			if tt.node != nil {
				require.NotNil(t, response.NodeError)
				assert.Equal(t, tt.node.Code, response.NodeError.Code)
			}

			assert.Empty(t, f.producer.Messages())
			assert.Equal(t, kindstest.Running, f.stored(t, "fan-uuid").Status.State)
		})
	}
}

func TestUseCase_Execute_waits(t *testing.T) {
	t.Parallel()

	ctx := context.Background()

	t.Run("a command that waits, asked of a resource in flight, has what it desires written down, and nothing is sent yet", func(t *testing.T) {
		t.Parallel()

		f := newFixture(t, kindstest.AFan("fan-uuid", kindstest.Starting, kindstest.Running))

		response, err := f.useCase.Execute(ctx, &actOnResource.Request{Kind: kindstest.Kind, UUID: "fan-uuid", Action: "stop"})
		require.NoError(t, err)

		assert.Empty(t, response.ValidationErrors)
		assert.Nil(t, response.Command, "it is asked once the fan runs")
		assert.Empty(t, f.producer.Messages())

		fan := f.stored(t, "fan-uuid")
		assert.Equal(t, kindstest.Starting, fan.Status.State, "it is left on its way")
		assert.Equal(t, kindstest.Stopped, fan.Status.Expected, "and is to be stopped once it gets there")
		assert.Equal(t, fan, kindstest.Typed(resource.Record{Raw: response.Resource}))
	})

	t.Run("nothing waits on a resource on its way to being deleted", func(t *testing.T) {
		t.Parallel()

		f := newFixture(t, kindstest.AFan("fan-uuid", kindstest.Deleting, kind.Deleted))

		response, err := f.useCase.Execute(ctx, &actOnResource.Request{Kind: kindstest.Kind, UUID: "fan-uuid", Action: "start"})
		require.NoError(t, err)

		assert.Equal(t, domain.ValidationErrors{"action": "invalid_state_transition"}, response.ValidationErrors)
		assert.Equal(t, kind.Deleted, f.stored(t, "fan-uuid").Status.Expected)
	})

	t.Run("and one at rest where it is not allowed is refused all the same", func(t *testing.T) {
		t.Parallel()

		f := newFixture(t, kindstest.AFan("fan-uuid", kindstest.Stopped, kindstest.Stopped))

		response, err := f.useCase.Execute(ctx, &actOnResource.Request{Kind: kindstest.Kind, UUID: "fan-uuid", Action: "stop"})
		require.NoError(t, err)

		assert.Equal(t, domain.ValidationErrors{"action": "invalid_state_transition"}, response.ValidationErrors)
	})

	t.Run("a command that does not wait is refused in flight", func(t *testing.T) {
		t.Parallel()

		f := newFixture(t, kindstest.AFan("fan-uuid", kindstest.Starting, kindstest.Running))

		response, err := f.useCase.Execute(ctx, &actOnResource.Request{Kind: kindstest.Kind, UUID: "fan-uuid", Action: "nudge"})
		require.NoError(t, err)

		assert.Equal(t, domain.ValidationErrors{"action": "invalid_state_transition"}, response.ValidationErrors)
	})
}

func TestUseCase_Execute_follows(t *testing.T) {
	t.Parallel()

	ctx := context.Background()

	t.Run("a command run in the control plane is followed at once by what the kind asks for of the resource as it now is", func(t *testing.T) {
		t.Parallel()

		f := newFixture(t, kindstest.AFan("fan-uuid", kindstest.Running, kindstest.Running))
		f.fans.Intents = func(fan kindstest.Fan) []kind.Intent {
			if fan.Metadata.Name == "hall" {
				return []kind.Intent{{Action: "stop", Reason: "a fan in the hall is too loud"}}
			}

			return nil
		}

		response, err := f.useCase.Execute(ctx, &actOnResource.Request{Kind: kindstest.Kind, UUID: "fan-uuid", Action: "rename", Payload: json.RawMessage(`{"name": "hall"}`)})
		require.NoError(t, err)

		require.NotNil(t, response.Command, "the stop it asked for is sent")
		assert.Equal(t, "stop", response.Command.Action)
		assert.Equal(t, "hall", response.Resource.Metadata.Name)

		sent, err := messagingMock.Produced[kind.Command](f.producer, kind.CommandName)
		require.NoError(t, err)
		require.Len(t, sent, 1)
		assert.Equal(t, "hall", sent[0].Resource.Metadata.Name, "carrying the resource as it was changed")

		fan := f.stored(t, "fan-uuid")
		assert.Equal(t, kindstest.Stopping, fan.Status.State)
		assert.Equal(t, "hall", fan.Metadata.Name)
	})

	t.Run("what the kind could not decide is the reconcile loop's to ask again, and the change stands", func(t *testing.T) {
		t.Parallel()

		f := newFixture(t, kindstest.AFan("fan-uuid", kindstest.Running, kindstest.Running))
		f.fans.Intents = func(kindstest.Fan) []kind.Intent {
			return []kind.Intent{{Action: "start"}}
		}

		response, err := f.useCase.Execute(ctx, &actOnResource.Request{Kind: kindstest.Kind, UUID: "fan-uuid", Action: "rename", Payload: json.RawMessage(`{"name": "hall"}`)})
		require.NoError(t, err)

		assert.Nil(t, response.Command, "a running fan cannot be started")
		assert.Equal(t, "hall", f.stored(t, "fan-uuid").Metadata.Name)
		assert.Empty(t, f.producer.Messages())
	})
}

func TestUseCase_Execute_extras(t *testing.T) {
	t.Parallel()

	ctx := context.Background()

	shelved := func(t *testing.T) (*actOnResource.UseCase, *kindstest.Shelf) {
		t.Helper()

		f := newFixture(t)
		s := kindstest.NewShelf(kindstest.AShelvedFan("shelved"))

		dispatcher := dispatch.New(f.memory, f.producer, f.waiters, nil)

		return actOnResource.NewUseCase(registry(t, &kindstest.Shelved{Fans: f.fans, Shelf: s}), f.memory, dispatcher, slog.New(slog.DiscardHandler)), s
	}

	shelvedFan := func(t *testing.T, s *kindstest.Shelf) (kindstest.Fan, bool) {
		t.Helper()

		r, err := s.One(context.Background(), "shelved")
		if errors.Is(err, domain.ErrNotExists) {
			return kindstest.Fan{}, false
		}

		require.NoError(t, err)

		return kindstest.Typed(resource.Record{Raw: r}), true
	}

	t.Run("a uuid that names none of the kind's records may name one of its extras, which says what came of it", func(t *testing.T) {
		t.Parallel()

		useCase, s := shelved(t)

		response, err := useCase.Execute(ctx, &actOnResource.Request{Kind: kindstest.Kind, UUID: "shelved", Action: "stop"})
		require.NoError(t, err)

		assert.Empty(t, response.ValidationErrors)
		assert.Equal(t, kindstest.Stopping, kindstest.Typed(resource.Record{Raw: response.Resource}).Status.State)

		fan, _ := shelvedFan(t, s)
		assert.Equal(t, kindstest.Stopping, fan.Status.State)
	})

	t.Run("one it takes away is gone", func(t *testing.T) {
		t.Parallel()

		useCase, s := shelved(t)

		response, err := useCase.Execute(ctx, &actOnResource.Request{Kind: kindstest.Kind, UUID: "shelved", Action: "delete"})
		require.NoError(t, err)

		assert.True(t, response.Gone)

		_, kept := shelvedFan(t, s)
		assert.False(t, kept)
	})

	t.Run("and what it cannot be asked is refused, field by field", func(t *testing.T) {
		t.Parallel()

		useCase, _ := shelved(t)

		response, err := useCase.Execute(ctx, &actOnResource.Request{Kind: kindstest.Kind, UUID: "shelved", Action: "start"})
		require.NoError(t, err)

		assert.Equal(t, domain.ValidationErrors{"fan": "immutable"}, response.ValidationErrors)
	})

	t.Run("an extra is nobody's own", func(t *testing.T) {
		t.Parallel()

		useCase, s := shelved(t)

		_, err := useCase.Execute(ctx, &actOnResource.Request{Kind: kindstest.Kind, OwnerUUID: kindstest.OwnerUUID, UUID: "shelved", Action: "stop"})

		assert.ErrorIs(t, err, domain.ErrNotExists)

		fan, _ := shelvedFan(t, s)
		assert.Equal(t, kindstest.Running, fan.Status.State)
	})

	t.Run("and a uuid that names nothing at all is not there", func(t *testing.T) {
		t.Parallel()

		useCase, _ := shelved(t)

		_, err := useCase.Execute(ctx, &actOnResource.Request{Kind: kindstest.Kind, UUID: "nowhere", Action: "stop"})

		assert.ErrorIs(t, err, domain.ErrNotExists)
	})
}
