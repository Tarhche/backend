package deleteResource_test

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/khanzadimahdi/testproject/application/workload/controlplane/kinds/deleteResource"
	"github.com/khanzadimahdi/testproject/application/workload/controlplane/kinds/dispatch"
	"github.com/khanzadimahdi/testproject/application/workload/controlplane/kinds/kindstest"
	"github.com/khanzadimahdi/testproject/application/workload/controlplane/kinds/waiters"
	"github.com/khanzadimahdi/testproject/domain"
	"github.com/khanzadimahdi/testproject/domain/workload/graph"
	"github.com/khanzadimahdi/testproject/domain/workload/graph/graphtest"
	"github.com/khanzadimahdi/testproject/domain/workload/kind"
	"github.com/khanzadimahdi/testproject/domain/workload/resource"
	messagingMock "github.com/khanzadimahdi/testproject/infrastructure/messaging/mock"
	resourcesMemory "github.com/khanzadimahdi/testproject/infrastructure/repository/memory/workload/resources"
)

type fixture struct {
	resources *resourcesMemory.Repository
	producer  *messagingMock.Recorder
	useCase   *deleteResource.UseCase
}

// newFixture is a delete over records, of the fan as descriptor has it.
func newFixture(t *testing.T, descriptor kind.Descriptor, records ...resource.Record) *fixture {
	t.Helper()

	return newFixtureWith(t, descriptor, nil, records...)
}

// newFixtureWith is a delete over records, of the fan as descriptor has it,
// as options say.
func newFixtureWith(t *testing.T, descriptor kind.Descriptor, options []deleteResource.Option, records ...resource.Record) *fixture {
	t.Helper()

	f := &fixture{resources: resourcesMemory.NewRepository(), producer: &messagingMock.Recorder{}}

	for _, r := range records {
		_, err := f.resources.Create(context.Background(), r)
		require.NoError(t, err)
	}

	registry := kind.NewRegistry[kind.ControlPlaneBinding]()
	require.NoError(t, registry.Register(kind.BindControlPlane[kindstest.Spec, kindstest.Status](descriptor, &kindstest.Fans{})))

	dispatcher := dispatch.New(f.resources, f.producer, waiters.New(), kindstest.NewClock().Now, dispatch.PollEvery(time.Millisecond))
	f.useCase = deleteResource.NewUseCase(registry, f.resources, dispatcher, options...)

	return f
}

func (f *fixture) stored(t *testing.T, uuid string) resource.Record {
	t.Helper()

	r, err := f.resources.GetOne(context.Background(), kindstest.Kind, uuid)
	require.NoError(t, err)

	return r
}

func TestUseCase_Execute(t *testing.T) {
	t.Parallel()

	ctx := context.Background()

	t.Run("a resource is deleted by its kind's delete, sent to its node", func(t *testing.T) {
		t.Parallel()

		f := newFixture(t, kindstest.Descriptor(), kindstest.AFan("fan-uuid", kindstest.Running, kindstest.Running))

		response, err := f.useCase.Execute(ctx, &deleteResource.Request{Kind: kindstest.Kind, OwnerUUID: kindstest.OwnerUUID, UUID: "fan-uuid"})
		require.NoError(t, err)

		require.NotNil(t, response.Command)
		assert.Equal(t, "delete", response.Command.Action)
		assert.False(t, response.Gone)

		stored := f.stored(t, "fan-uuid")
		fan := kindstest.Typed(stored)
		assert.Equal(t, kindstest.Deleting, fan.Status.State)
		assert.Equal(t, kind.Deleted, fan.Status.Expected)
		assert.Equal(t, kindstest.Moment, fan.Metadata.DeletionRequestedAt, "its delete requested now")
		assert.Equal(t, int64(2), stored.Version, "in the write that expects it deleted")

		assert.Equal(t, []string{kind.ActOnResourceName}, f.producer.Subjects())
	})

	t.Run("one asked to go before was requested to when it first was", func(t *testing.T) {
		t.Parallel()

		before := kindstest.Moment.Add(-time.Hour)

		f := newFixture(t, kindstest.Descriptor(), kindstest.AFan("fan-uuid", kind.Failed, kind.Deleted, func(fan *kindstest.Fan) {
			fan.Metadata.DeletionRequestedAt = before
		}))

		response, err := f.useCase.Execute(ctx, &deleteResource.Request{Kind: kindstest.Kind, UUID: "fan-uuid"})
		require.NoError(t, err)

		require.NotNil(t, response.Command, "its delete asked again")
		assert.Equal(t, before, f.stored(t, "fan-uuid").Metadata.DeletionRequestedAt)
	})

	t.Run("its kind's delete is asked with what it is asked with", func(t *testing.T) {
		t.Parallel()

		// a fan deleted at a speed.
		d := kindstest.Descriptor()
		for i := range d.Actions {
			if d.Actions[i].Name == "delete" {
				d.Actions[i].Payload = kind.Payload[kindstest.StartPayload]()
			}
		}

		f := newFixture(t, d, kindstest.AFan("fan-uuid", kindstest.Running, kindstest.Running))

		response, err := f.useCase.Execute(ctx, &deleteResource.Request{Kind: kindstest.Kind, UUID: "fan-uuid", Payload: []byte(`{"speed": 2}`)})
		require.NoError(t, err)

		require.NotNil(t, response.Command)
		assert.JSONEq(t, `{"speed": 2}`, string(response.Command.Payload))

		g := newFixture(t, d, kindstest.AFan("fan-uuid", kindstest.Running, kindstest.Running))

		refused, err := g.useCase.Execute(ctx, &deleteResource.Request{Kind: kindstest.Kind, UUID: "fan-uuid", Payload: []byte(`{"speed": 9}`)})
		require.NoError(t, err)

		assert.Equal(t, domain.ValidationErrors{"speed": "invalid_value"}, refused.ValidationErrors, "and what its codec refuses of it is said, rather than nothing asked")
		assert.Nil(t, refused.Command)
		assert.Empty(t, g.producer.Messages())
		assert.Equal(t, kindstest.Running, kindstest.Typed(g.stored(t, "fan-uuid")).Status.State, "and it is left as it was")
	})

	t.Run("one being deleted already is left to it", func(t *testing.T) {
		t.Parallel()

		record := kindstest.AFan("fan-uuid", kindstest.Deleting, kind.Deleted)
		record.Pending = &resource.Pending{Action: "delete", IDs: []string{"command-1"}}

		f := newFixture(t, kindstest.Descriptor(), record)

		response, err := f.useCase.Execute(ctx, &deleteResource.Request{Kind: kindstest.Kind, UUID: "fan-uuid"})
		require.NoError(t, err)

		assert.Nil(t, response.Command)
		assert.Equal(t, kindstest.Deleting, kindstest.Typed(resource.Record{Raw: response.Resource}).Status.State)
		assert.Empty(t, f.producer.Messages())
		assert.Equal(t, int64(1), f.stored(t, "fan-uuid").Version)
	})

	t.Run("one with nothing anywhere to delete goes at once", func(t *testing.T) {
		t.Parallel()

		f := newFixture(t, kindstest.Descriptor(), kindstest.AFan("fan-uuid", kindstest.Pending, kindstest.Running, func(fan *kindstest.Fan) {
			fan.Metadata.Node = ""
		}))

		response, err := f.useCase.Execute(ctx, &deleteResource.Request{Kind: kindstest.Kind, UUID: "fan-uuid", Wait: time.Minute})
		require.NoError(t, err)

		assert.True(t, response.Gone)
		assert.Equal(t, 0, f.resources.Len(kindstest.Kind))
	})

	t.Run("one whose state does not allow it yet is expected deleted, and asked once it can be", func(t *testing.T) {
		t.Parallel()

		// a fan deleted only once it is at rest.
		atRest := []kind.State{kindstest.Running, kindstest.Stopped, kind.Failed, kind.Missing}

		d := kindstest.Descriptor()
		for i := range d.Actions {
			if d.Actions[i].Name == "delete" {
				d.Actions[i].AllowedIn = atRest
			}
		}

		var transitions []kind.Transition
		for _, transition := range d.Machine.Transitions {
			if transition.On != kind.OnAction("delete") {
				transitions = append(transitions, transition)
			}
		}

		for _, from := range atRest {
			transitions = append(transitions, kind.Transition{From: from, On: kind.OnAction("delete"), To: kindstest.Deleting})
		}

		d.Machine.Transitions = transitions

		f := newFixture(t, d, kindstest.AFan("fan-uuid", kindstest.Starting, kindstest.Running))

		response, err := f.useCase.Execute(ctx, &deleteResource.Request{Kind: kindstest.Kind, UUID: "fan-uuid"})
		require.NoError(t, err)

		assert.Nil(t, response.Command, "nothing is asked of it yet")

		fan := kindstest.Typed(f.stored(t, "fan-uuid"))
		assert.Equal(t, kindstest.Starting, fan.Status.State)
		assert.Equal(t, kind.Deleted, fan.Status.Expected)
		assert.Equal(t, kindstest.Moment, fan.Metadata.DeletionRequestedAt, "though its delete was requested now")
		assert.Empty(t, f.producer.Messages())
	})

	for name, tt := range map[string]struct {
		request deleteResource.Request
		err     error
	}{
		"somebody else's is not there": {
			request: deleteResource.Request{Kind: kindstest.Kind, OwnerUUID: "somebody-else", UUID: "fan-uuid"},
			err:     domain.ErrNotExists,
		},
		"nor is one that is not": {
			request: deleteResource.Request{Kind: kindstest.Kind, UUID: "another-uuid"},
			err:     domain.ErrNotExists,
		},
		"nor one of a kind not run here": {
			request: deleteResource.Request{Kind: "kettle", UUID: "fan-uuid"},
			err:     kind.ErrUnknownKind,
		},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			f := newFixture(t, kindstest.Descriptor(), kindstest.AFan("fan-uuid", kindstest.Running, kindstest.Running))

			_, err := f.useCase.Execute(ctx, &tt.request)

			assert.ErrorIs(t, err, tt.err)
			assert.Empty(t, f.producer.Messages())
		})
	}
}

func TestUseCase_Execute_extras(t *testing.T) {
	t.Parallel()

	ctx := context.Background()

	shelved := func(t *testing.T, fans ...kindstest.Fan) (*deleteResource.UseCase, *kindstest.Shelf) {
		t.Helper()

		shelf := kindstest.NewShelf(fans...)
		resources := resourcesMemory.NewRepository()

		registry := kind.NewRegistry[kind.ControlPlaneBinding]()
		require.NoError(t, registry.Register(kind.BindControlPlane[kindstest.Spec, kindstest.Status](kindstest.Descriptor(), &kindstest.Shelved{Fans: &kindstest.Fans{}, Shelf: shelf})))

		dispatcher := dispatch.New(resources, &messagingMock.Recorder{}, waiters.New(), nil)

		return deleteResource.NewUseCase(registry, resources, dispatcher), shelf
	}

	t.Run("a uuid that names none of the kind's records may name one of its extras, which takes itself away", func(t *testing.T) {
		t.Parallel()

		useCase, shelf := shelved(t, kindstest.AShelvedFan("shelved"))

		response, err := useCase.Execute(ctx, &deleteResource.Request{Kind: kindstest.Kind, UUID: "shelved"})
		require.NoError(t, err)

		assert.True(t, response.Gone)
		assert.Empty(t, response.ValidationErrors)

		_, err = shelf.One(ctx, "shelved")
		assert.ErrorIs(t, err, domain.ErrNotExists)
	})

	t.Run("which is whose it says: somebody else asking for their own does not take it away", func(t *testing.T) {
		t.Parallel()

		useCase, shelf := shelved(t, kindstest.AShelvedFan("shelved"))

		_, err := useCase.Execute(ctx, &deleteResource.Request{Kind: kindstest.Kind, OwnerUUID: kindstest.OwnerUUID, UUID: "shelved"})
		assert.ErrorIs(t, err, domain.ErrNotExists)

		_, err = shelf.One(ctx, "shelved")
		assert.NoError(t, err)
	})

	t.Run("nor does a request about a parent it does not live in", func(t *testing.T) {
		t.Parallel()

		useCase, shelf := shelved(t, kindstest.AShelvedFan("shelved"))

		_, err := useCase.Execute(ctx, &deleteResource.Request{Kind: kindstest.Kind, UUID: "shelved", Parent: kindstest.House})
		assert.ErrorIs(t, err, domain.ErrNotExists)

		_, err = shelf.One(ctx, "shelved")
		assert.NoError(t, err)
	})

	t.Run("and one that names neither is not there", func(t *testing.T) {
		t.Parallel()

		useCase, _ := shelved(t)

		_, err := useCase.Execute(ctx, &deleteResource.Request{Kind: kindstest.Kind, UUID: "nowhere"})

		assert.ErrorIs(t, err, domain.ErrNotExists)
	})
}

// TestUseCase_Execute_roots holds the delete of a root its graph removes to
// leaving it to its graph: marked, queued to its graph, and waited on until
// its graph has removed it, while its graph's own delete of it, and the delete
// of what is no root, is asked of its node.
func TestUseCase_Execute_roots(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	root := graph.Ref{Kind: kindstest.Kind, UUID: "fan-uuid"}

	// rooted is a delete over records, a fan being a root its graph
	// removes, which is queued to told.
	rooted := func(t *testing.T, descriptor kind.Descriptor, told *graphtest.Told, records ...resource.Record) *fixture {
		t.Helper()

		return newFixtureWith(t, descriptor, []deleteResource.Option{deleteResource.WithGraphs(graph.Roots{kindstest.Kind}, told)}, records...)
	}

	t.Run("a root is marked to be deleted rather than asked, and queued to its graph, which removes it", func(t *testing.T) {
		t.Parallel()

		told := &graphtest.Told{}
		f := rooted(t, kindstest.Descriptor(), told, kindstest.AFan("fan-uuid", kindstest.Running, kindstest.Running))

		response, err := f.useCase.Execute(ctx, &deleteResource.Request{Kind: kindstest.Kind, UUID: "fan-uuid"})
		require.NoError(t, err)

		assert.Nil(t, response.Command, "nothing is asked of its node")
		assert.False(t, response.Gone)
		assert.Empty(t, f.producer.Messages())

		stored := f.stored(t, "fan-uuid")
		fan := kindstest.Typed(stored)

		assert.Equal(t, kind.Deleted, fan.Status.Expected)
		assert.Equal(t, kindstest.Running, fan.Status.State)
		assert.Equal(t, kindstest.Moment, stored.Metadata.DeletionRequestedAt)
		assert.Equal(t, stored.Raw, response.Resource)

		assert.Equal(t, []graph.Ref{root}, told.Queued())
	})

	t.Run("and is gone once its graph has removed it, when that is waited for", func(t *testing.T) {
		t.Parallel()

		f := rooted(t, kindstest.Descriptor(), &graphtest.Told{}, kindstest.AFan("fan-uuid", kindstest.Running, kindstest.Running))

		go func() {
			time.Sleep(20 * time.Millisecond)
			_ = f.resources.Delete(context.Background(), kindstest.Kind, "fan-uuid")
		}()

		response, err := f.useCase.Execute(ctx, &deleteResource.Request{Kind: kindstest.Kind, UUID: "fan-uuid", Wait: time.Minute})
		require.NoError(t, err)

		assert.True(t, response.Gone)
	})

	t.Run("what its kind refuses of its delete is refused, and it is neither marked nor queued", func(t *testing.T) {
		t.Parallel()

		// a fan deleted at a speed.
		d := kindstest.Descriptor()
		for i := range d.Actions {
			if d.Actions[i].Name == "delete" {
				d.Actions[i].Payload = kind.Payload[kindstest.StartPayload]()
			}
		}

		told := &graphtest.Told{}
		f := rooted(t, d, told, kindstest.AFan("fan-uuid", kindstest.Running, kindstest.Running))

		response, err := f.useCase.Execute(ctx, &deleteResource.Request{Kind: kindstest.Kind, UUID: "fan-uuid", Payload: []byte(`{"speed": 9}`)})
		require.NoError(t, err)

		assert.Equal(t, domain.ValidationErrors{"speed": "invalid_value"}, response.ValidationErrors)
		assert.Equal(t, int64(1), f.stored(t, "fan-uuid").Version)
		assert.Empty(t, told.Queued())
	})

	for name, tt := range map[string]struct {
		record  resource.Record
		byGraph bool
	}{
		"its graph's own delete of it is asked of its node": {
			record:  kindstest.AFan("fan-uuid", kindstest.Running, kind.Deleted),
			byGraph: true,
		},
		"and so is the delete of what something owns, which is no root": {
			record: kindstest.AFan("fan-uuid", kindstest.Running, kindstest.Running, func(fan *kindstest.Fan) {
				fan.Metadata.Owners = []kind.Reference{{Kind: kindstest.Kind, UUID: "another-fan"}}
			}),
		},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			told := &graphtest.Told{}
			f := rooted(t, kindstest.Descriptor(), told, tt.record)

			response, err := f.useCase.Execute(ctx, &deleteResource.Request{Kind: kindstest.Kind, UUID: "fan-uuid", ByGraph: tt.byGraph})
			require.NoError(t, err)

			require.NotNil(t, response.Command)
			assert.Equal(t, "delete", response.Command.Action)
			assert.Equal(t, kindstest.Deleting, kindstest.Typed(f.stored(t, "fan-uuid")).Status.State)
			assert.Empty(t, told.Queued())
		})
	}
}

// TestUseCase_Execute_dependents holds the delete of what the graphs of roots
// need to answering with them, and whether one makes it again, and, deleted
// with them, to marking them to go first and waiting for them to: a root's
// own delete, and its graph's, are as they were.
func TestUseCase_Execute_dependents(t *testing.T) {
	t.Parallel()

	ctx := context.Background()

	needed := graph.Ref{Kind: kindstest.Kind, UUID: "needed"}
	web := graph.Ref{Kind: kindstest.Kind, UUID: "web"}
	needs := []graph.Dependent{{Root: web, Name: "kitchen", Recreates: true}}

	// owned has a fan owned by another fan, which is no root.
	owned := func(fan *kindstest.Fan) {
		fan.Metadata.Owners = []kind.Reference{{Kind: kindstest.Kind, UUID: "owner"}}
	}

	// rooted is a delete over records, fans being roots of their graphs, the
	// root web needing needed as told says.
	rooted := func(t *testing.T, told *graphtest.Told, records ...resource.Record) *fixture {
		t.Helper()

		return newFixtureWith(t, kindstest.Descriptor(), []deleteResource.Option{deleteResource.WithGraphs(graph.Roots{kindstest.Kind}, told)}, records...)
	}

	// removing has the root's graph remove what refs name once it is
	// queued, a moment after, as its graph would.
	removing := func(f *fixture, told *graphtest.Told, refs ...graph.Ref) {
		go func() {
			for len(told.Queued()) == 0 {
				time.Sleep(time.Millisecond)
			}

			time.Sleep(20 * time.Millisecond)

			for _, ref := range refs {
				_ = f.resources.Delete(context.Background(), ref.Kind, ref.UUID)
			}
		}()
	}

	t.Run("what roots need is asked of its node as anything is, and answered with them, which make it again", func(t *testing.T) {
		t.Parallel()

		told := &graphtest.Told{Depending: map[graph.Ref][]graph.Dependent{needed: needs}}
		f := rooted(t, told,
			kindstest.AFan("needed", kindstest.Running, kindstest.Running, owned),
			kindstest.AFan("web", kindstest.Running, kindstest.Running),
		)

		response, err := f.useCase.Execute(ctx, &deleteResource.Request{Kind: kindstest.Kind, UUID: "needed"})
		require.NoError(t, err)

		require.NotNil(t, response.Command)
		assert.Equal(t, "delete", response.Command.Action)
		assert.Equal(t, needs, response.Dependents)
		assert.True(t, response.Recreated)

		assert.Equal(t, kindstest.Running, kindstest.Typed(f.stored(t, "web")).Status.Expected, "the root is left as it was")
		assert.Empty(t, told.Queued())
	})

	t.Run("one that only waits on it does not make it again", func(t *testing.T) {
		t.Parallel()

		told := &graphtest.Told{Depending: map[graph.Ref][]graph.Dependent{needed: {{Root: web, Name: "kitchen"}}}}
		f := rooted(t, told, kindstest.AFan("needed", kindstest.Running, kindstest.Running, owned))

		response, err := f.useCase.Execute(ctx, &deleteResource.Request{Kind: kindstest.Kind, UUID: "needed"})
		require.NoError(t, err)

		assert.Len(t, response.Dependents, 1)
		assert.False(t, response.Recreated)
	})

	t.Run("with cascade, they are marked to go first, waited on to go, and then it is asked", func(t *testing.T) {
		t.Parallel()

		told := &graphtest.Told{Depending: map[graph.Ref][]graph.Dependent{needed: needs}}
		f := rooted(t, told,
			kindstest.AFan("needed", kindstest.Running, kindstest.Running, owned),
			kindstest.AFan("web", kindstest.Running, kindstest.Running),
		)

		// what the root was once it was queued, and whether anything was
		// asked of a node by then.
		seen := make(chan bool, 1)

		go func() {
			for len(told.Queued()) == 0 {
				time.Sleep(time.Millisecond)
			}

			r, err := f.resources.GetOne(context.Background(), web.Kind, web.UUID)
			marked := err == nil && kindstest.Typed(r).Status.Expected == kind.Deleted && !r.Metadata.DeletionRequestedAt.IsZero()
			seen <- marked && len(f.producer.Messages()) == 0
		}()

		removing(f, told, web)

		// waiting for nothing, its roots are waited on as long as a cascade
		// waits when it is told nothing, and its own delete not at all.
		response, err := f.useCase.Execute(ctx, &deleteResource.Request{Kind: kindstest.Kind, UUID: "needed", Cascade: true})
		require.NoError(t, err)

		assert.True(t, <-seen, "the root was marked, as a delete by force would, before anything was asked")
		assert.Equal(t, []graph.Ref{web}, told.Queued())

		_, err = f.resources.GetOne(ctx, web.Kind, web.UUID)
		assert.ErrorIs(t, err, domain.ErrNotExists, "it went first")

		require.NotNil(t, response.Command, "and then it was asked")
		assert.Nil(t, response.Result, "and not waited on")
		assert.Equal(t, needs, response.Dependents)
		assert.False(t, response.Recreated, "nothing makes it again once they go")
	})

	t.Run("and once what took it with them is done, it is gone", func(t *testing.T) {
		t.Parallel()

		told := &graphtest.Told{Depending: map[graph.Ref][]graph.Dependent{needed: needs}}
		f := rooted(t, told,
			kindstest.AFan("needed", kindstest.Running, kindstest.Running, owned),
			kindstest.AFan("web", kindstest.Running, kindstest.Running),
		)

		removing(f, told, needed, web)

		response, err := f.useCase.Execute(ctx, &deleteResource.Request{Kind: kindstest.Kind, UUID: "needed", Cascade: true, Wait: time.Minute})
		require.NoError(t, err)

		assert.True(t, response.Gone)
		assert.Nil(t, response.Command)
		assert.Equal(t, needs, response.Dependents)
	})

	t.Run("what refuses its delete is answered with them beside it", func(t *testing.T) {
		t.Parallel()

		// a fan deleted at a speed.
		d := kindstest.Descriptor()
		for i := range d.Actions {
			if d.Actions[i].Name == "delete" {
				d.Actions[i].Payload = kind.Payload[kindstest.StartPayload]()
			}
		}

		told := &graphtest.Told{Depending: map[graph.Ref][]graph.Dependent{needed: needs}}
		f := newFixtureWith(t, d, []deleteResource.Option{deleteResource.WithGraphs(graph.Roots{kindstest.Kind}, told)}, kindstest.AFan("needed", kindstest.Running, kindstest.Running, owned))

		response, err := f.useCase.Execute(ctx, &deleteResource.Request{Kind: kindstest.Kind, UUID: "needed", Payload: []byte(`{"speed": 9}`)})
		require.NoError(t, err)

		assert.Equal(t, domain.ValidationErrors{"speed": "invalid_value"}, response.ValidationErrors)
		assert.Equal(t, needs, response.Dependents)
	})

	t.Run("a root's own delete is as it was, cascade or not, and its graph's own asks of nothing", func(t *testing.T) {
		t.Parallel()

		told := &graphtest.Told{Depending: map[graph.Ref][]graph.Dependent{web: needs, needed: needs}}
		f := rooted(t, told,
			kindstest.AFan("web", kindstest.Running, kindstest.Running),
			kindstest.AFan("needed", kindstest.Running, kind.Deleted, owned),
		)

		response, err := f.useCase.Execute(ctx, &deleteResource.Request{Kind: kindstest.Kind, UUID: "web", Cascade: true})
		require.NoError(t, err)

		assert.Nil(t, response.Command, "marked, not asked")
		assert.Empty(t, response.Dependents)
		assert.Equal(t, []graph.Ref{web}, told.Queued())

		response, err = f.useCase.Execute(ctx, &deleteResource.Request{Kind: kindstest.Kind, UUID: "needed", ByGraph: true})
		require.NoError(t, err)

		require.NotNil(t, response.Command)
		assert.Empty(t, response.Dependents)
		assert.Empty(t, told.Asked(), "which roots need either is not asked")
	})
}
