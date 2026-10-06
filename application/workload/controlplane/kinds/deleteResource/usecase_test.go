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

	f := &fixture{resources: resourcesMemory.NewRepository(), producer: &messagingMock.Recorder{}}

	for _, r := range records {
		_, err := f.resources.Create(context.Background(), r)
		require.NoError(t, err)
	}

	registry := kind.NewRegistry[kind.ControlPlaneBinding]()
	require.NoError(t, registry.Register(kind.BindControlPlane[kindstest.Spec, kindstest.Status](descriptor, &kindstest.Fans{})))

	dispatcher := dispatch.New(f.resources, f.producer, waiters.New(), kindstest.NewClock().Now, dispatch.PollEvery(time.Millisecond))
	f.useCase = deleteResource.NewUseCase(registry, f.resources, dispatcher)

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

		fan := kindstest.Typed(f.stored(t, "fan-uuid"))
		assert.Equal(t, kindstest.Deleting, fan.Status.State)
		assert.Equal(t, kind.Deleted, fan.Status.Expected)

		assert.Equal(t, []string{kind.CommandName}, f.producer.Subjects())
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

	t.Run("which is nobody's own", func(t *testing.T) {
		t.Parallel()

		useCase, shelf := shelved(t, kindstest.AShelvedFan("shelved"))

		_, err := useCase.Execute(ctx, &deleteResource.Request{Kind: kindstest.Kind, OwnerUUID: "guest", UUID: "shelved"})
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
