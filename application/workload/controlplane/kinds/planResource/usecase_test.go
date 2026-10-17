package planResource_test

import (
	"context"
	"log/slog"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/khanzadimahdi/testproject/application/workload/controlplane/graph/execute"
	"github.com/khanzadimahdi/testproject/application/workload/controlplane/kinds/admitResource"
	"github.com/khanzadimahdi/testproject/application/workload/controlplane/kinds/dispatch"
	"github.com/khanzadimahdi/testproject/application/workload/controlplane/kinds/kindstest"
	"github.com/khanzadimahdi/testproject/application/workload/controlplane/kinds/planResource"
	"github.com/khanzadimahdi/testproject/application/workload/controlplane/kinds/waiters"
	"github.com/khanzadimahdi/testproject/domain"
	"github.com/khanzadimahdi/testproject/domain/workload/graph"
	"github.com/khanzadimahdi/testproject/domain/workload/graph/graphtest"
	"github.com/khanzadimahdi/testproject/domain/workload/kind"
	"github.com/khanzadimahdi/testproject/domain/workload/resource"
	messagingMock "github.com/khanzadimahdi/testproject/infrastructure/messaging/mock"
	resourcesMemory "github.com/khanzadimahdi/testproject/infrastructure/repository/memory/workload/resources"
)

// previews are the graphs as a dry run sees them: what needs what is what
// Told says, and what carrying a root's graph out, or removing it, would do
// is one node, the root, with the verdict each says, and each root it was
// asked of is kept.
type previews struct {
	graphtest.Told

	lock    sync.Mutex
	created []kind.Raw
	removed []graph.Ref
}

var _ planResource.Graphs = &previews{}

func (p *previews) PreviewCreate(_ context.Context, root kind.Raw) (execute.Preview, error) {
	p.lock.Lock()
	defer p.lock.Unlock()

	p.created = append(p.created, root)

	return execute.Preview{Levels: [][]execute.Previewed{{{Node: graph.ID(root.Kind, root.Metadata.UUID), Kind: root.Kind, Verdict: execute.Create}}}}, nil
}

func (p *previews) PreviewRemove(_ context.Context, root graph.Ref) (execute.Preview, error) {
	p.lock.Lock()
	defer p.lock.Unlock()

	p.removed = append(p.removed, root)

	return execute.Preview{Levels: [][]execute.Previewed{{{Node: graph.ID(root.Kind, root.UUID), Kind: root.Kind, Verdict: execute.Delete, Ref: root}}}}, nil
}

type fixture struct {
	resources *resourcesMemory.Repository
	producer  *messagingMock.Recorder
	useCase   *planResource.UseCase
}

// newFixture is a dry run over fans kept in memory, the roots of their own
// graphs, which graphs previews, when there are graphs.
func newFixture(t *testing.T, graphs planResource.Graphs, records ...resource.Record) *fixture {
	t.Helper()

	f := &fixture{resources: resourcesMemory.NewRepository(), producer: &messagingMock.Recorder{}}

	for _, r := range records {
		_, err := f.resources.Create(context.Background(), r)
		require.NoError(t, err)
	}

	registry := kindstest.Registry(&kindstest.Fans{Node: kindstest.NodeName})
	dispatcher := dispatch.New(f.resources, f.producer, waiters.New(), kindstest.NewClock().Now, dispatch.PollEvery(time.Millisecond))
	admission := admitResource.NewUseCase(registry, f.resources, dispatcher, slog.New(slog.DiscardHandler))

	f.useCase = planResource.NewUseCase(registry, f.resources, admission, graph.Roots{kindstest.Kind}, graphs)

	return f
}

func (f *fixture) all(t *testing.T) []resource.Record {
	t.Helper()

	records, _, err := f.resources.GetAll(context.Background(), kindstest.Kind, resource.Filter{}, 0, 0)
	require.NoError(t, err)

	return records
}

// asked is a fan asked for, of so many blades, owned by owners.
func asked(blades string, owners ...kind.Reference) planResource.Request {
	return planResource.Request{
		Kind:      kindstest.Kind,
		OwnerUUID: kindstest.OwnerUUID,
		Manifest: kind.Raw{
			Metadata: kind.Metadata{Name: "kitchen", Owners: owners},
			Spec:     []byte(`{"blades": ` + blades + `, "house": "house-1"}`),
		},
	}
}

// owned has a fan owned by another fan, which is no root.
func owned(fan *kindstest.Fan) {
	fan.Metadata.Owners = []kind.Reference{{Kind: kindstest.Kind, UUID: "owner"}}
}

func TestUseCase_Execute_admission(t *testing.T) {
	t.Parallel()

	ctx := context.Background()

	t.Run("a root is answered as it would be admitted, and with what carrying out its graph would do, and nothing is kept or sent", func(t *testing.T) {
		t.Parallel()

		graphs := &previews{}
		f := newFixture(t, graphs)

		request := asked("3")

		response, err := f.useCase.Execute(ctx, &request)
		require.NoError(t, err)
		require.Empty(t, response.ValidationErrors)

		fan := kindstest.Typed(resource.Record{Raw: response.Resource})
		assert.NotEmpty(t, fan.Metadata.UUID)
		assert.Equal(t, "kitchen", fan.Metadata.Name)

		require.NotNil(t, response.Plan)
		assert.Equal(t, graph.ID(kindstest.Kind, fan.Metadata.UUID), response.Plan.Levels[0][0].Node)

		require.Len(t, graphs.created, 1)
		assert.Equal(t, response.Resource, graphs.created[0], "its graph is planned from what would be kept")

		assert.Empty(t, f.all(t), "nothing is kept")
		assert.Empty(t, f.producer.Messages(), "nor sent")
		assert.Empty(t, graphs.Queued(), "nor queued")
	})

	t.Run("what is no root is answered as it would be admitted, and nothing more", func(t *testing.T) {
		t.Parallel()

		graphs := &previews{}
		f := newFixture(t, graphs)

		request := asked("3", kind.Reference{Kind: kindstest.Kind, UUID: "owner"})

		response, err := f.useCase.Execute(ctx, &request)
		require.NoError(t, err)

		assert.Nil(t, response.Plan)
		assert.Empty(t, graphs.created)
		assert.Empty(t, f.all(t))
	})

	t.Run("what would be refused is refused", func(t *testing.T) {
		t.Parallel()

		graphs := &previews{}
		f := newFixture(t, graphs)

		request := asked("0")

		response, err := f.useCase.Execute(ctx, &request)
		require.NoError(t, err)

		assert.Equal(t, domain.ValidationErrors{"blades": "required_field"}, response.ValidationErrors)
		assert.Empty(t, graphs.created)
	})

	t.Run("where no graphs are kept, nothing is a root", func(t *testing.T) {
		t.Parallel()

		f := newFixture(t, nil)

		request := asked("3")

		response, err := f.useCase.Execute(ctx, &request)
		require.NoError(t, err)

		assert.NotEmpty(t, response.Resource.Metadata.UUID)
		assert.Nil(t, response.Plan)
		assert.Empty(t, f.all(t))
	})
}

func TestUseCase_Execute_delete(t *testing.T) {
	t.Parallel()

	ctx := context.Background()

	root := graph.Ref{Kind: kindstest.Kind, UUID: "web"}
	needed := graph.Ref{Kind: kindstest.Kind, UUID: "needed"}

	t.Run("a root is answered with what removing it would do, and nothing changes", func(t *testing.T) {
		t.Parallel()

		graphs := &previews{}
		f := newFixture(t, graphs, kindstest.AFan("web", kindstest.Running, kindstest.Running))

		response, err := f.useCase.Execute(ctx, &planResource.Request{Kind: kindstest.Kind, Delete: true, UUID: "web", Cascade: true})
		require.NoError(t, err)

		require.NotNil(t, response.Plan)
		assert.Equal(t, root, response.Plan.Levels[0][0].Ref)
		assert.Empty(t, response.Dependents)
		assert.Empty(t, graphs.Asked(), "which roots need a root is not asked")

		stored, err := f.resources.GetOne(ctx, kindstest.Kind, "web")
		require.NoError(t, err)
		assert.Equal(t, int64(1), stored.Version, "nothing changes")
		assert.Empty(t, graphs.Queued())
	})

	t.Run("what is no root is answered with the roots that need it, and whether one would make it again", func(t *testing.T) {
		t.Parallel()

		graphs := &previews{Told: graphtest.Told{Depending: map[graph.Ref][]graph.Dependent{needed: {{Root: root, Name: "kitchen", Recreates: true}}}}}
		f := newFixture(t, graphs, kindstest.AFan("needed", kindstest.Running, kindstest.Running, owned))

		response, err := f.useCase.Execute(ctx, &planResource.Request{Kind: kindstest.Kind, Delete: true, UUID: "needed"})
		require.NoError(t, err)

		assert.Nil(t, response.Plan)
		assert.Equal(t, []planResource.Dependent{{Dependent: graph.Dependent{Root: root, Name: "kitchen", Recreates: true}}}, response.Dependents)
		assert.True(t, response.Recreated)
		assert.Empty(t, graphs.removed, "what removing them would do is not asked without cascade")
	})

	t.Run("and, with cascade, with what removing each would do, none of which would make it again", func(t *testing.T) {
		t.Parallel()

		graphs := &previews{Told: graphtest.Told{Depending: map[graph.Ref][]graph.Dependent{needed: {{Root: root, Name: "kitchen", Recreates: true}}}}}
		f := newFixture(t, graphs, kindstest.AFan("needed", kindstest.Running, kindstest.Running, owned))

		response, err := f.useCase.Execute(ctx, &planResource.Request{Kind: kindstest.Kind, Delete: true, UUID: "needed", Cascade: true})
		require.NoError(t, err)

		require.Len(t, response.Dependents, 1)
		require.NotNil(t, response.Dependents[0].Plan)
		assert.Equal(t, root, response.Dependents[0].Plan.Levels[0][0].Ref)
		assert.False(t, response.Recreated)
		assert.Equal(t, []graph.Ref{root}, graphs.removed)
		assert.Empty(t, graphs.Queued(), "nothing is marked or queued")
	})

	t.Run("what is not there, or not the owner's, is not there", func(t *testing.T) {
		t.Parallel()

		f := newFixture(t, &previews{}, kindstest.AFan("web", kindstest.Running, kindstest.Running))

		_, err := f.useCase.Execute(ctx, &planResource.Request{Kind: kindstest.Kind, Delete: true, UUID: "nowhere"})
		assert.ErrorIs(t, err, domain.ErrNotExists)

		_, err = f.useCase.Execute(ctx, &planResource.Request{Kind: kindstest.Kind, OwnerUUID: "somebody-else", Delete: true, UUID: "web"})
		assert.ErrorIs(t, err, domain.ErrNotExists)
	})

	t.Run("a kind that is not run here is not one", func(t *testing.T) {
		t.Parallel()

		_, err := newFixture(t, &previews{}).useCase.Execute(ctx, &planResource.Request{Kind: "lamp", Delete: true, UUID: "web"})
		assert.ErrorIs(t, err, kind.ErrUnknownKind)
	})
}
