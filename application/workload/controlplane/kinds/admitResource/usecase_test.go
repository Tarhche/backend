package admitResource_test

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/khanzadimahdi/testproject/application/workload/controlplane/kinds/admitResource"
	"github.com/khanzadimahdi/testproject/application/workload/controlplane/kinds/dispatch"
	"github.com/khanzadimahdi/testproject/application/workload/controlplane/kinds/kindstest"
	"github.com/khanzadimahdi/testproject/application/workload/controlplane/kinds/waiters"
	"github.com/khanzadimahdi/testproject/domain"
	"github.com/khanzadimahdi/testproject/domain/workload/kind"
	"github.com/khanzadimahdi/testproject/domain/workload/resource"
	messagingMock "github.com/khanzadimahdi/testproject/infrastructure/messaging/mock"
	resourcesMemory "github.com/khanzadimahdi/testproject/infrastructure/repository/memory/workload/resources"
)

type fixture struct {
	resources *resourcesMemory.Repository
	producer  *messagingMock.Recorder
	waiters   *waiters.Waiters
	clock     *kindstest.Clock
	fans      *kindstest.Fans
	useCase   *admitResource.UseCase
}

func newFixture(registry func(fans *kindstest.Fans) *kind.Registry[kind.ControlPlaneBinding]) *fixture {
	f := &fixture{
		resources: resourcesMemory.NewRepository(),
		producer:  &messagingMock.Recorder{},
		waiters:   waiters.New(),
		clock:     kindstest.NewClock(),
		fans:      &kindstest.Fans{Node: kindstest.NodeName},
	}

	if registry == nil {
		registry = kindstest.Registry
	}

	dispatcher := dispatch.New(f.resources, f.producer, f.waiters, f.clock.Now, dispatch.PollEvery(time.Millisecond))
	f.useCase = admitResource.NewUseCase(registry(f.fans), f.resources, dispatcher, slog.New(slog.DiscardHandler))

	return f
}

func (f *fixture) all(t *testing.T) []resource.Record {
	t.Helper()

	records, _, err := f.resources.GetAll(context.Background(), kindstest.Kind, resource.Filter{}, 0, 0)
	require.NoError(t, err)

	return records
}

// passedOver keeps resources as its repository does, and has a pass of the
// reconcile loop come upon each one the moment it is kept.
type passedOver struct {
	*resourcesMemory.Repository

	pass func(r resource.Record)
}

func (p *passedOver) Create(ctx context.Context, r resource.Record) (resource.Record, error) {
	created, err := p.Repository.Create(ctx, r)
	if err == nil {
		p.pass(created)
	}

	return created, err
}

// answer writes down what came of the command the one resource kept is
// waiting on, once it waits on one, as another control plane hearing its
// node would: it runs, said output.
func answer(f *fixture, output string) {
	for {
		records, _, _ := f.resources.GetAll(context.Background(), kindstest.Kind, resource.Filter{}, 0, 0)
		if len(records) == 1 && records[0].Pending != nil {
			stored := records[0]
			stored.Answer = &kind.ResourceActedOn{ID: stored.Pending.IDs[0], Action: stored.Pending.Action, OK: true, Output: output}
			stored.Pending = nil

			common, _ := stored.Common()
			common.State = kindstest.Running
			_ = stored.SetCommon(common)

			if _, err := f.resources.Update(context.Background(), stored); err == nil {
				return
			}
		}

		time.Sleep(time.Millisecond)
	}
}

func asked(name string, spec string) admitResource.Request {
	return admitResource.Request{
		Kind:      kindstest.Kind,
		OwnerUUID: kindstest.OwnerUUID,
		Manifest: kind.Raw{
			Metadata: kind.Metadata{Name: name, Labels: map[string]string{"room": "kitchen"}},
			Spec:     json.RawMessage(spec),
		},
	}
}

func TestUseCase_Execute(t *testing.T) {
	t.Parallel()

	ctx := context.Background()

	t.Run("a resource is admitted by its kind, kept, and sent the first command its kind asks for", func(t *testing.T) {
		t.Parallel()

		f := newFixture(nil)

		request := asked("kitchen", `{"blades": 3, "house": "house-1"}`)
		request.Manifest.Metadata.UUID = "chosen-by-the-caller"
		request.Manifest.Metadata.Node = "a node of the caller's"
		request.Manifest.Metadata.OwnerUUID = "somebody else"
		request.Manifest.Metadata.Lifetime = time.Hour

		response, err := f.useCase.Execute(ctx, &request)
		require.NoError(t, err)
		require.Empty(t, response.ValidationErrors)

		records := f.all(t)
		require.Len(t, records, 1)

		stored := records[0]
		fan := kindstest.Typed(stored)

		assert.NotEqual(t, "chosen-by-the-caller", stored.Metadata.UUID, "which it is is not the caller's to say")
		assert.NotEmpty(t, stored.Metadata.UUID)
		assert.Equal(t, kindstest.Kind, stored.Kind)
		assert.Equal(t, "kitchen", stored.Metadata.Name)
		assert.Equal(t, "kitchen-fan", stored.Metadata.Slug, "its kind gave it a slug")
		assert.Equal(t, kindstest.OwnerUUID, stored.Metadata.OwnerUUID, "it is whom it was asked for's")
		assert.Equal(t, kindstest.NodeName, stored.Metadata.Node, "and where its kind placed it")
		assert.Equal(t, map[string]string{"room": "kitchen"}, stored.Metadata.Labels)
		assert.Equal(t, []kind.Reference{{Kind: kindstest.Parent, UUID: "house-1"}}, stored.Metadata.Owners)
		assert.Equal(t, kindstest.Moment, stored.Metadata.CreatedAt)
		assert.Equal(t, kindstest.Moment.Add(time.Hour), stored.Metadata.ExpiresAt, "its lifetime ends an hour from now")

		assert.Equal(t, kindstest.Spec{Blades: 3, House: "house-1"}, fan.Spec)
		assert.Equal(t, kindstest.Starting, fan.Status.State, "it is on its way to what its first command desires")
		assert.Equal(t, kindstest.Running, fan.Status.Expected)

		require.NotNil(t, response.Command)
		assert.Equal(t, "create", response.Command.Action, "an internal command is the workload's own to ask for")
		assert.Equal(t, stored.Metadata.UUID, response.Command.UUID)
		assert.Equal(t, stored.Raw, response.Resource)
		assert.Nil(t, response.Result, "nothing was waited for")

		sent, err := messagingMock.Produced[kind.ActOnResource](f.producer, kind.ActOnResourceName)
		require.NoError(t, err)
		require.Len(t, sent, 1)
		assert.Equal(t, response.Command.ID, sent[0].ID)
		assert.Equal(t, kindstest.NodeName, sent[0].Node)

		require.NotNil(t, stored.Pending, "the command is what it waits on")
		assert.Equal(t, []string{response.Command.ID}, stored.Pending.IDs)
	})

	t.Run("one of a kind that asks for nothing is only kept", func(t *testing.T) {
		t.Parallel()

		f := newFixture(nil)
		f.fans.Intents = func(kindstest.Fan) []kind.Intent { return nil }

		request := asked("kitchen", `{"blades": 3}`)

		response, err := f.useCase.Execute(ctx, &request)
		require.NoError(t, err)

		assert.Nil(t, response.Command)
		assert.Equal(t, kindstest.Pending, kindstest.Typed(f.all(t)[0]).Status.State, "it is in the state its machine starts it in")
		assert.Empty(t, f.producer.Messages())
	})

	t.Run("the resource it lives in may be named on its own", func(t *testing.T) {
		t.Parallel()

		f := newFixture(nil)

		request := asked("kitchen", `{"blades": 3}`)
		request.Parent = "house-2"

		_, err := f.useCase.Execute(ctx, &request)
		require.NoError(t, err)

		assert.Equal(t, []kind.Reference{{Kind: kindstest.Parent, UUID: "house-2"}}, f.all(t)[0].Metadata.Owners)
	})

	t.Run("what came of its first command is waited for, when asked to be", func(t *testing.T) {
		t.Parallel()

		f := newFixture(nil)

		request := asked("kitchen", `{"blades": 3}`)
		request.Wait = time.Minute

		// the node's answer, heard and written down by another control
		// plane while this one waits.
		go func() {
			for {
				records, _, _ := f.resources.GetAll(context.Background(), kindstest.Kind, resource.Filter{}, 0, 0)
				if len(records) == 1 && records[0].Pending != nil {
					stored := records[0]
					stored.Answer = &kind.ResourceActedOn{ID: stored.Pending.IDs[0], Action: "create", OK: true, Output: "made"}
					stored.Pending = nil

					common, _ := stored.Common()
					common.State = kindstest.Running
					_ = stored.SetCommon(common)

					if _, err := f.resources.Update(context.Background(), stored); err == nil {
						return
					}
				}

				time.Sleep(time.Millisecond)
			}
		}()

		response, err := f.useCase.Execute(ctx, &request)
		require.NoError(t, err)

		require.NotNil(t, response.Result)
		assert.Equal(t, "made", response.Result.Output)
		assert.Equal(t, kindstest.Running, kindstest.Typed(resource.Record{Raw: response.Resource}).Status.State, "it is as the answer left it")
	})

	for name, tt := range map[string]struct {
		request func() admitResource.Request
		invalid domain.ValidationErrors
	}{
		"a resource is asked for somebody": {
			request: func() admitResource.Request {
				request := asked("kitchen", `{"blades": 3}`)
				request.OwnerUUID = ""

				return request
			},
			invalid: domain.ValidationErrors{"owner": "required_field"},
		},
		"of the kind it is asked as": {
			request: func() admitResource.Request {
				request := asked("kitchen", `{"blades": 3}`)
				request.Manifest.Kind = "kettle"

				return request
			},
			invalid: domain.ValidationErrors{"kind": "invalid_value"},
		},
		"for no less than no time": {
			request: func() admitResource.Request {
				request := asked("kitchen", `{"blades": 3}`)
				request.Manifest.Metadata.Lifetime = -time.Second

				return request
			},
			invalid: domain.ValidationErrors{"lifetime": "invalid_value"},
		},
		"what its kind refuses is refused": {
			request: func() admitResource.Request { return asked("kitchen", `{"blades": 0}`) },
			invalid: domain.ValidationErrors{"blades": "required_field"},
		},
		"and so is a spec that is not its kind's": {
			request: func() admitResource.Request { return asked("kitchen", `{"blades": "three"}`) },
			invalid: domain.ValidationErrors{"spec": "invalid_value"},
		},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			f := newFixture(nil)
			request := tt.request()

			response, err := f.useCase.Execute(ctx, &request)
			require.NoError(t, err)

			assert.Equal(t, tt.invalid, response.ValidationErrors)
			assert.Empty(t, f.all(t), "nothing is kept")
			assert.Empty(t, f.producer.Messages())
		})
	}

	t.Run("a resource of a kind with no parent lives in nothing", func(t *testing.T) {
		t.Parallel()

		f := newFixture(func(fans *kindstest.Fans) *kind.Registry[kind.ControlPlaneBinding] {
			d := kindstest.Descriptor()
			d.Parent = ""
			d.OnParent = kind.ParentRules{}

			registry := kind.NewRegistry[kind.ControlPlaneBinding]()
			if err := registry.Register(kind.BindControlPlane[kindstest.Spec, kindstest.Status](d, fans)); err != nil {
				panic(err)
			}

			return registry
		})

		request := asked("kitchen", `{"blades": 3}`)
		request.Parent = "house-1"

		response, err := f.useCase.Execute(ctx, &request)
		require.NoError(t, err)

		assert.Equal(t, domain.ValidationErrors{"parent": "invalid_value"}, response.ValidationErrors)
	})

	t.Run("one given a slug another took in the meantime is admitted again, and given another", func(t *testing.T) {
		t.Parallel()

		f := newFixture(nil)
		f.fans.Slugs = []string{"kitchen-fan", "kitchen-fan", "kitchen-abcde"}

		first := asked("kitchen", `{"blades": 3}`)
		_, err := f.useCase.Execute(ctx, &first)
		require.NoError(t, err)

		second := asked("kitchen", `{"blades": 3}`)
		response, err := f.useCase.Execute(ctx, &second)
		require.NoError(t, err)

		assert.Equal(t, "kitchen-abcde", response.Resource.Metadata.Slug)
		assert.Len(t, f.all(t), 2)
	})

	t.Run("but not for ever", func(t *testing.T) {
		t.Parallel()

		f := newFixture(nil)
		f.fans.Slugs = []string{"kitchen-fan"}

		first := asked("kitchen", `{"blades": 3}`)
		_, err := f.useCase.Execute(ctx, &first)
		require.NoError(t, err)

		second := asked("kitchen", `{"blades": 3}`)
		_, err = f.useCase.Execute(ctx, &second)

		assert.ErrorIs(t, err, domain.ErrAlreadyExists)
		assert.Len(t, f.all(t), 1)
	})

	t.Run("what kept its kind from looking at it is an error", func(t *testing.T) {
		t.Parallel()

		f := newFixture(nil)
		f.fans.Failure = errors.New("the database is gone")

		request := asked("kitchen", `{"blades": 3}`)

		_, err := f.useCase.Execute(ctx, &request)

		assert.ErrorIs(t, err, f.fans.Failure)
	})

	t.Run("a kind not run here is not asked", func(t *testing.T) {
		t.Parallel()

		f := newFixture(nil)

		request := asked("kitchen", `{"blades": 3}`)
		request.Kind = "kettle"

		_, err := f.useCase.Execute(ctx, &request)

		assert.ErrorIs(t, err, kind.ErrUnknownKind)
	})

	t.Run("one its first command cannot be sent to is kept, and sent it later", func(t *testing.T) {
		t.Parallel()

		f := newFixture(nil)
		f.producer.Err = errors.New("nats is away")

		request := asked("kitchen", `{"blades": 3}`)

		response, err := f.useCase.Execute(ctx, &request)
		require.NoError(t, err)

		require.NotNil(t, response.Command)
		require.Len(t, f.all(t), 1)
		assert.NotNil(t, f.all(t)[0].Pending, "the reconcile loop sends what it waits on again")
	})

	t.Run("one the reconcile loop asks for its first command before it is asked is as the loop left it", func(t *testing.T) {
		t.Parallel()

		f := newFixture(nil)
		registry := kindstest.Registry(f.fans)
		binding, _ := registry.Lookup(kindstest.Kind)

		// the loop's pass, which comes upon the resource the moment it is kept
		// and asks it for what its kind asks, as admitting it is about to.
		loop := dispatch.New(f.resources, f.producer, f.waiters, f.clock.Now)
		kept := &passedOver{Repository: f.resources, pass: func(r resource.Record) {
			asked, err := loop.Follow(ctx, binding, r)
			require.NoError(t, err)
			require.NotNil(t, asked.Command)

			_, err = loop.Send(ctx, *asked.Command, 0)
			require.NoError(t, err)
		}}

		admitting := admitResource.NewUseCase(registry, kept, dispatch.New(kept, f.producer, f.waiters, f.clock.Now, dispatch.PollEvery(time.Millisecond)), slog.New(slog.DiscardHandler))

		request := asked("kitchen", `{"blades": 3}`)
		request.Wait = time.Minute

		// the node's answer to what the loop sent, heard and written down by
		// another control plane.
		go answer(f, "made for the loop")

		response, err := admitting.Execute(ctx, &request)
		require.NoError(t, err)
		require.Empty(t, response.ValidationErrors)

		sent, err := messagingMock.Produced[kind.ActOnResource](f.producer, kind.ActOnResourceName)
		require.NoError(t, err)
		require.Len(t, sent, 1, "what the loop sent is not sent again")
		assert.Equal(t, "create", sent[0].Action)

		assert.Nil(t, response.Command, "it was the loop's to send")
		require.NotNil(t, response.Result, "what came of it is what was waited for")
		assert.Equal(t, "made for the loop", response.Result.Output)
		assert.Equal(t, kindstest.Running, kindstest.Typed(resource.Record{Raw: response.Resource}).Status.State, "it is as the answer left it, never as it was before the loop came upon it")
	})

	t.Run("and one whose answer is written down before its admission reads it again is answered with it", func(t *testing.T) {
		t.Parallel()

		f := newFixture(nil)
		registry := kindstest.Registry(f.fans)
		binding, _ := registry.Lookup(kindstest.Kind)

		// the loop's pass sends it its first command, and its node answers at
		// once, which another control plane writes down: all before admitting
		// it follows it.
		loop := dispatch.New(f.resources, f.producer, f.waiters, f.clock.Now)
		kept := &passedOver{Repository: f.resources, pass: func(r resource.Record) {
			asked, err := loop.Follow(ctx, binding, r)
			require.NoError(t, err)
			require.NotNil(t, asked.Command)

			_, err = loop.Send(ctx, *asked.Command, 0)
			require.NoError(t, err)

			answer(f, "made before it was looked for")
		}}

		admitting := admitResource.NewUseCase(registry, kept, dispatch.New(kept, f.producer, f.waiters, f.clock.Now, dispatch.PollEvery(time.Millisecond)), slog.New(slog.DiscardHandler))

		request := asked("kitchen", `{"blades": 3}`)
		request.Wait = time.Minute

		response, err := admitting.Execute(ctx, &request)
		require.NoError(t, err)

		sent, err := messagingMock.Produced[kind.ActOnResource](f.producer, kind.ActOnResourceName)
		require.NoError(t, err)
		require.Len(t, sent, 1, "what the loop sent is not sent again")

		assert.Nil(t, response.Command)
		require.NotNil(t, response.Result, "what came of it is what was waited for, though it came first")
		assert.Equal(t, "made before it was looked for", response.Result.Output)
		assert.Equal(t, kindstest.Running, kindstest.Typed(resource.Record{Raw: response.Resource}).Status.State)
	})

	t.Run("and not waited for, it is as the loop left it all the same", func(t *testing.T) {
		t.Parallel()

		f := newFixture(nil)
		registry := kindstest.Registry(f.fans)
		binding, _ := registry.Lookup(kindstest.Kind)

		loop := dispatch.New(f.resources, f.producer, f.waiters, f.clock.Now)
		kept := &passedOver{Repository: f.resources, pass: func(r resource.Record) {
			_, err := loop.Follow(ctx, binding, r)
			require.NoError(t, err)
		}}

		admitting := admitResource.NewUseCase(registry, kept, dispatch.New(kept, f.producer, f.waiters, f.clock.Now), slog.New(slog.DiscardHandler))

		request := asked("kitchen", `{"blades": 3}`)

		response, err := admitting.Execute(ctx, &request)
		require.NoError(t, err)

		assert.Equal(t, kindstest.Starting, kindstest.Typed(resource.Record{Raw: response.Resource}).Status.State, "on its way to what its first command desires")
		assert.Nil(t, response.Result)
	})

	t.Run("and one placed on no node is kept waiting for one", func(t *testing.T) {
		t.Parallel()

		f := newFixture(nil)
		f.fans.Node = ""

		request := asked("kitchen", `{"blades": 3}`)

		response, err := f.useCase.Execute(ctx, &request)
		require.NoError(t, err)

		assert.Nil(t, response.Command)
		assert.Equal(t, kindstest.Pending, kindstest.Typed(f.all(t)[0]).Status.State)
		assert.Empty(t, f.producer.Messages())
	})
}
