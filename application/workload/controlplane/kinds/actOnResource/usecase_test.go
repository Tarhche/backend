package actOnResource_test

import (
	"context"
	"encoding/json"
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
	useCase  *actOnResource.UseCase
}

func newFixture(t *testing.T, records ...resource.Record) *fixture {
	t.Helper()

	f := &fixture{
		memory:   resourcesMemory.NewRepository(),
		producer: &messagingMock.Recorder{},
		waiters:  waiters.New(),
	}

	f.racing = &kindstest.Racing{Repository: f.memory}

	for _, r := range records {
		_, err := f.memory.Create(context.Background(), r)
		require.NoError(t, err)
	}

	dispatcher := dispatch.New(f.racing, f.producer, f.waiters, kindstest.NewClock().Now, dispatch.PollEvery(time.Millisecond))
	f.useCase = actOnResource.NewUseCase(kindstest.Registry(&kindstest.Fans{Node: kindstest.NodeName}), f.racing, dispatcher)

	return f
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
		"a command for a node, of one on none, cannot be sent": {
			request:  actOnResource.Request{Action: "stop"},
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
