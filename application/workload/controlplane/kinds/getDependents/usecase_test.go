package getDependents_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/khanzadimahdi/testproject/application/workload/controlplane/kinds/dependents"
	"github.com/khanzadimahdi/testproject/application/workload/controlplane/kinds/dispatch"
	"github.com/khanzadimahdi/testproject/application/workload/controlplane/kinds/getDependents"
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

// the records the tests are about: needed, a fan another fan owns, which is
// no root, and the roots that need it, fans nothing owns.
var (
	needed = graph.Ref{Kind: kindstest.Kind, UUID: "needed"}
	web    = graph.Ref{Kind: kindstest.Kind, UUID: "web"}
	api    = graph.Ref{Kind: kindstest.Kind, UUID: "api"}
)

// fixture is which roots need fans kept in memory, fans being roots of their
// own graphs, which told says need what it says they need; fans are found in
// their house by their names, and those on the shelf beside them.
type fixture struct {
	memory   *resourcesMemory.Repository
	told     *graphtest.Told
	produced *messagingMock.Recorder
	useCase  *getDependents.UseCase
}

func newFixture(t *testing.T, told *graphtest.Told, shelf *kindstest.Shelf, records ...resource.Record) *fixture {
	t.Helper()

	f := &fixture{memory: resourcesMemory.NewRepository(), told: told, produced: &messagingMock.Recorder{}}

	for _, r := range records {
		_, err := f.memory.Create(context.Background(), r)
		require.NoError(t, err)
	}

	if shelf == nil {
		shelf = kindstest.NewShelf()
	}

	registry := kind.NewRegistry[kind.ControlPlaneBinding]()
	require.NoError(t, registry.Register(kind.BindControlPlane[kindstest.Spec, kindstest.Status](kindstest.Descriptor(), &kindstest.Witnessing{Fans: &kindstest.Fans{}, Records: f.memory, Shelf: shelf})))

	dispatcher := dispatch.New(f.memory, f.produced, waiters.New(), kindstest.NewClock().Now, dispatch.PollEvery(time.Millisecond))

	var graphs graph.Graphs
	if told != nil {
		graphs = told
	}

	f.useCase = getDependents.NewUseCase(registry, f.memory, dependents.New(registry, f.memory, dispatcher, graph.Roots{kindstest.Kind}, graphs))

	return f
}

// owned has a fan owned by another fan, which is no root.
func owned(fan *kindstest.Fan) {
	fan.Metadata.Owners = []kind.Reference{{Kind: kindstest.Kind, UUID: "owner"}}
}

func TestUseCase_Execute(t *testing.T) {
	t.Parallel()

	ctx := context.Background()

	t.Run("what is no root is needed by the roots the graphs say, and nothing is changed, queued or sent for asking", func(t *testing.T) {
		t.Parallel()

		needs := []graph.Dependent{{Root: web, Name: "kitchen", Recreates: true}, {Root: api, Name: "hall"}}

		told := &graphtest.Told{Depending: map[graph.Ref][]graph.Dependent{needed: needs}}
		f := newFixture(t, told, nil, kindstest.AFan("needed", kindstest.Running, kindstest.Running, owned))

		before, err := f.memory.GetOne(ctx, needed.Kind, needed.UUID)
		require.NoError(t, err)

		response, err := f.useCase.Execute(ctx, &getDependents.Request{Kind: kindstest.Kind, OwnerUUID: kindstest.OwnerUUID, UUID: "needed"})
		require.NoError(t, err)

		assert.Equal(t, needs, response.Dependents)
		assert.True(t, response.Recreated, "one of them makes it again")
		assert.Equal(t, []graph.Ref{needed}, told.Asked())

		after, err := f.memory.GetOne(ctx, needed.Kind, needed.UUID)
		require.NoError(t, err)
		assert.Equal(t, before, after, "it is as it was")
		assert.Empty(t, told.Queued(), "nothing is queued")
		assert.Empty(t, told.Changes())
		assert.Empty(t, f.produced.Messages(), "and nothing is sent to a node")
	})

	t.Run("one only waited on is made again by nothing", func(t *testing.T) {
		t.Parallel()

		told := &graphtest.Told{Depending: map[graph.Ref][]graph.Dependent{needed: {{Root: web, Name: "kitchen"}}}}
		f := newFixture(t, told, nil, kindstest.AFan("needed", kindstest.Running, kindstest.Running, owned))

		response, err := f.useCase.Execute(ctx, &getDependents.Request{Kind: kindstest.Kind, UUID: "needed"})
		require.NoError(t, err)

		assert.Len(t, response.Dependents, 1)
		assert.False(t, response.Recreated)
	})

	t.Run("what nothing needs is needed by none", func(t *testing.T) {
		t.Parallel()

		f := newFixture(t, &graphtest.Told{}, nil, kindstest.AFan("needed", kindstest.Running, kindstest.Running, owned))

		response, err := f.useCase.Execute(ctx, &getDependents.Request{Kind: kindstest.Kind, UUID: "needed"})
		require.NoError(t, err)

		assert.Empty(t, response.Dependents)
		assert.False(t, response.Recreated)
	})

	t.Run("a root is needed by nothing, and the graphs are not asked", func(t *testing.T) {
		t.Parallel()

		told := &graphtest.Told{Depending: map[graph.Ref][]graph.Dependent{web: {{Root: api, Recreates: true}}}}
		f := newFixture(t, told, nil, kindstest.AFan("web", kindstest.Running, kindstest.Running))

		response, err := f.useCase.Execute(ctx, &getDependents.Request{Kind: kindstest.Kind, UUID: "web"})
		require.NoError(t, err)

		assert.Empty(t, response.Dependents)
		assert.Empty(t, told.Asked())
	})

	t.Run("where the graphs of roots are not kept, nothing needs anything", func(t *testing.T) {
		t.Parallel()

		f := newFixture(t, nil, nil, kindstest.AFan("needed", kindstest.Running, kindstest.Running, owned))

		response, err := f.useCase.Execute(ctx, &getDependents.Request{Kind: kindstest.Kind, UUID: "needed"})
		require.NoError(t, err)

		assert.Empty(t, response.Dependents)
	})

	t.Run("inside its house, it is found by its name, as its delete finds it, and nowhere else", func(t *testing.T) {
		t.Parallel()

		told := &graphtest.Told{Depending: map[graph.Ref][]graph.Dependent{needed: {{Root: web, Recreates: true}}}}
		f := newFixture(t, told, nil, kindstest.AFan("needed", kindstest.Running, kindstest.Running, owned, func(fan *kindstest.Fan) {
			fan.Metadata.Name = "ceiling"
		}))

		response, err := f.useCase.Execute(ctx, &getDependents.Request{Kind: kindstest.Kind, UUID: "ceiling", Parent: kindstest.House})
		require.NoError(t, err)
		assert.Len(t, response.Dependents, 1)

		_, err = f.useCase.Execute(ctx, &getDependents.Request{Kind: kindstest.Kind, UUID: "ceiling", Parent: "house-2"})
		assert.ErrorIs(t, err, domain.ErrNotExists)
	})

	t.Run("what nobody keeps a record of is needed by nothing, to whoever may see it", func(t *testing.T) {
		t.Parallel()

		told := &graphtest.Told{}
		f := newFixture(t, told, kindstest.NewShelf(kindstest.AShelvedFan("shelved")))

		response, err := f.useCase.Execute(ctx, &getDependents.Request{Kind: kindstest.Kind, OwnerUUID: "guest", UUID: "shelved"})
		require.NoError(t, err)

		assert.Empty(t, response.Dependents)
		assert.False(t, response.Recreated)
		assert.Empty(t, told.Asked(), "and the graphs are not asked of it")

		_, err = f.useCase.Execute(ctx, &getDependents.Request{Kind: kindstest.Kind, OwnerUUID: "somebody-else", UUID: "shelved"})
		assert.ErrorIs(t, err, domain.ErrNotExists, "somebody else's is not there")
	})

	for name, request := range map[string]getDependents.Request{
		"what is not there is not there":    {Kind: kindstest.Kind, UUID: "nowhere"},
		"nor is somebody else's":            {Kind: kindstest.Kind, OwnerUUID: "somebody-else", UUID: "needed"},
		"nor one that lives somewhere else": {Kind: kindstest.Kind, UUID: "needed", Parent: "house-2"},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			f := newFixture(t, &graphtest.Told{}, nil, kindstest.AFan("needed", kindstest.Running, kindstest.Running, owned))

			_, err := f.useCase.Execute(ctx, &request)
			assert.ErrorIs(t, err, domain.ErrNotExists)
		})
	}

	t.Run("a kind that is not run here is not one", func(t *testing.T) {
		t.Parallel()

		_, err := newFixture(t, nil, nil).useCase.Execute(ctx, &getDependents.Request{Kind: "lamp", UUID: "needed"})
		assert.ErrorIs(t, err, kind.ErrUnknownKind)
	})

	t.Run("what asking the graphs fails with is what it fails with", func(t *testing.T) {
		t.Parallel()

		failure := errors.New("the database is gone")

		told := &graphtest.Told{DependentsFailure: failure}
		f := newFixture(t, told, nil, kindstest.AFan("needed", kindstest.Running, kindstest.Running, owned))

		_, err := f.useCase.Execute(ctx, &getDependents.Request{Kind: kindstest.Kind, UUID: "needed"})
		assert.ErrorIs(t, err, failure)
	})
}
