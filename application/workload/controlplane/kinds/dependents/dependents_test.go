package dependents_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/khanzadimahdi/testproject/application/workload/controlplane/kinds/dependents"
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

// the records the tests are about: needed, a fan another fan owns, which is
// no root, and the roots that need it, fans nothing owns, all in the house.
var (
	needed = graph.Ref{Kind: kindstest.Kind, UUID: "needed"}
	web    = graph.Ref{Kind: kindstest.Kind, UUID: "web"}
	api    = graph.Ref{Kind: kindstest.Kind, UUID: "api"}
	house  = graph.Ref{Kind: kindstest.Parent, UUID: kindstest.House}
)

// fixture is dependents over fans kept in memory, roots of their own graphs,
// which told says need what it says they need.
type fixture struct {
	memory     *resourcesMemory.Repository
	told       *graphtest.Told
	dependents *dependents.Dependents
}

func newFixture(t *testing.T, told *graphtest.Told, records ...resource.Record) *fixture {
	t.Helper()

	f := &fixture{memory: resourcesMemory.NewRepository(), told: told}

	for _, r := range records {
		_, err := f.memory.Create(context.Background(), r)
		require.NoError(t, err)
	}

	registry := kind.NewRegistry[kind.ControlPlaneBinding]()
	require.NoError(t, registry.Register(kind.BindControlPlane[kindstest.Spec, kindstest.Status](kindstest.Descriptor(), &kindstest.Fans{})))

	dispatcher := dispatch.New(f.memory, &messagingMock.Recorder{}, waiters.New(), kindstest.NewClock().Now, dispatch.PollEvery(time.Millisecond))

	var graphs graph.Graphs
	if told != nil {
		graphs = told
	}

	f.dependents = dependents.New(registry, f.memory, dispatcher, graph.Roots{kindstest.Kind}, graphs)

	return f
}

// owned has a fan owned by another fan, which is no root.
func owned(fan *kindstest.Fan) {
	fan.Metadata.Owners = []kind.Reference{{Kind: kindstest.Kind, UUID: "owner"}}
}

// marked has a fan asked to go already.
func marked(fan *kindstest.Fan) {
	fan.Status.Expected = kind.Deleted
	fan.Metadata.DeletionRequestedAt = kindstest.Moment.Add(-time.Hour)
}

// livingElsewhere has a fan live in another house.
func livingElsewhere(fan *kindstest.Fan) {
	fan.Metadata.Refs = []kind.Reference{{Kind: kindstest.Parent, UUID: "house-2"}}
}

func (f *fixture) stored(t *testing.T, ref graph.Ref) (kindstest.Fan, bool) {
	t.Helper()

	r, err := f.memory.GetOne(context.Background(), ref.Kind, ref.UUID)
	if errors.Is(err, domain.ErrNotExists) {
		return kindstest.Fan{}, false
	}

	require.NoError(t, err)

	return kindstest.Typed(r), true
}

func TestDependents_Of(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	needs := []graph.Dependent{{Root: web, Name: "kitchen", Recreates: true}}

	t.Run("what is no root is needed by what the graphs say", func(t *testing.T) {
		t.Parallel()

		told := &graphtest.Told{Depending: map[graph.Ref][]graph.Dependent{needed: needs}}
		f := newFixture(t, told, kindstest.AFan("needed", kindstest.Running, kindstest.Running, owned))

		r, err := f.memory.GetOne(ctx, needed.Kind, needed.UUID)
		require.NoError(t, err)

		dependents, err := f.dependents.Of(ctx, kindstest.Descriptor(), r)
		require.NoError(t, err)

		assert.Equal(t, needs, dependents)
		assert.Equal(t, []graph.Ref{needed}, told.Asked())
	})

	t.Run("a root is needed by nothing, and the graphs are not asked", func(t *testing.T) {
		t.Parallel()

		told := &graphtest.Told{Depending: map[graph.Ref][]graph.Dependent{web: needs}}
		f := newFixture(t, told, kindstest.AFan("web", kindstest.Running, kindstest.Running))

		r, err := f.memory.GetOne(ctx, web.Kind, web.UUID)
		require.NoError(t, err)

		dependents, err := f.dependents.Of(ctx, kindstest.Descriptor(), r)
		require.NoError(t, err)

		assert.Empty(t, dependents)
		assert.Empty(t, told.Asked())
	})

	t.Run("nor is anything where no graphs are kept", func(t *testing.T) {
		t.Parallel()

		f := newFixture(t, nil, kindstest.AFan("needed", kindstest.Running, kindstest.Running, owned))

		r, err := f.memory.GetOne(ctx, needed.Kind, needed.UUID)
		require.NoError(t, err)

		dependents, err := f.dependents.Of(ctx, kindstest.Descriptor(), r)
		require.NoError(t, err)
		assert.Empty(t, dependents)
	})

	t.Run("what the graphs fail with is what it fails with", func(t *testing.T) {
		t.Parallel()

		told := &graphtest.Told{DependentsFailure: errors.New("the database is gone")}
		f := newFixture(t, told, kindstest.AFan("needed", kindstest.Running, kindstest.Running, owned))

		r, err := f.memory.GetOne(ctx, needed.Kind, needed.UUID)
		require.NoError(t, err)

		_, err = f.dependents.Of(ctx, kindstest.Descriptor(), r)
		assert.ErrorIs(t, err, told.DependentsFailure)
	})
}

func TestDependents_Mark(t *testing.T) {
	t.Parallel()

	ctx := context.Background()

	told := &graphtest.Told{}
	f := newFixture(t, told,
		kindstest.AFan("web", kindstest.Running, kindstest.Running),
		kindstest.AFan("api", kindstest.Stopped, kind.Deleted, marked),
	)

	gone := graph.Ref{Kind: kindstest.Kind, UUID: "gone"}

	require.NoError(t, f.dependents.Mark(ctx, []graph.Dependent{{Root: web}, {Root: api}, {Root: gone}}))

	t.Run("a root that runs is marked to go, as a delete by force would, and nothing refuses it", func(t *testing.T) {
		t.Parallel()

		fan, there := f.stored(t, web)
		require.True(t, there)

		assert.Equal(t, kind.Deleted, fan.Status.Expected)
		assert.Equal(t, kindstest.Running, fan.Status.State, "and its graph removes it")
		assert.Equal(t, kindstest.Moment, fan.Metadata.DeletionRequestedAt)
	})

	t.Run("one asked to go before keeps when it first was", func(t *testing.T) {
		t.Parallel()

		fan, there := f.stored(t, api)
		require.True(t, there)

		assert.Equal(t, kindstest.Moment.Add(-time.Hour), fan.Metadata.DeletionRequestedAt)
	})

	t.Run("each there is queued to its graph, and what is gone is gone already", func(t *testing.T) {
		t.Parallel()

		assert.Equal(t, []graph.Ref{web, api}, told.Queued())
	})
}

func TestDependents_Cascade(t *testing.T) {
	t.Parallel()

	ctx := context.Background()

	t.Run("those that do not live in what they need are waited on to go, and what is left of the wait is its own", func(t *testing.T) {
		t.Parallel()

		told := &graphtest.Told{}
		f := newFixture(t, told,
			kindstest.AFan("needed", kindstest.Running, kindstest.Running, owned),
			kindstest.AFan("web", kindstest.Running, kindstest.Running),
		)

		r, err := f.memory.GetOne(ctx, needed.Kind, needed.UUID)
		require.NoError(t, err)

		// its graph removes it a moment after it is queued.
		go func() {
			for len(told.Queued()) == 0 {
				time.Sleep(time.Millisecond)
			}

			time.Sleep(20 * time.Millisecond)
			_ = f.memory.Delete(context.Background(), web.Kind, web.UUID)
		}()

		left, err := f.dependents.Cascade(ctx, r, []graph.Dependent{{Root: web, Recreates: true}}, time.Minute)
		require.NoError(t, err)

		_, there := f.stored(t, web)
		assert.False(t, there, "it went before")
		assert.Greater(t, left, 50*time.Second)
		assert.LessOrEqual(t, left, time.Minute)
	})

	t.Run("one that lives in what it needs goes with it, and is not waited on", func(t *testing.T) {
		t.Parallel()

		told := &graphtest.Told{}
		f := newFixture(t, told, kindstest.AFan("web", kindstest.Running, kindstest.Running))

		houseRecord := resource.Record{Raw: kind.Raw{Kind: house.Kind, Metadata: kind.Metadata{UUID: house.UUID}}}

		began := time.Now()

		left, err := f.dependents.Cascade(ctx, houseRecord, []graph.Dependent{{Root: web, Recreates: true}}, time.Minute)
		require.NoError(t, err)

		assert.Less(t, time.Since(began), time.Second, "nothing was waited on")
		assert.Greater(t, left, 55*time.Second)
		assert.Equal(t, []graph.Ref{web}, told.Queued(), "it is marked and queued all the same")

		fan, there := f.stored(t, web)
		require.True(t, there)
		assert.Equal(t, kind.Deleted, fan.Status.Expected)
	})

	t.Run("one that lives in another house is waited on, until the wait is over", func(t *testing.T) {
		t.Parallel()

		f := newFixture(t, &graphtest.Told{}, kindstest.AFan("web", kindstest.Running, kindstest.Running, livingElsewhere))

		houseRecord := resource.Record{Raw: kind.Raw{Kind: house.Kind, Metadata: kind.Metadata{UUID: house.UUID}}}

		began := time.Now()

		left, err := f.dependents.Cascade(ctx, houseRecord, []graph.Dependent{{Root: web, Recreates: true}}, 50*time.Millisecond)
		require.NoError(t, err)

		assert.GreaterOrEqual(t, time.Since(began), 50*time.Millisecond)
		assert.Zero(t, left, "nothing is left of the wait")
	})

	t.Run("nothing asked to be waited for leaves nothing for its own delete to wait", func(t *testing.T) {
		t.Parallel()

		f := newFixture(t, &graphtest.Told{}, kindstest.AFan("web", kindstest.Running, kindstest.Running))

		houseRecord := resource.Record{Raw: kind.Raw{Kind: house.Kind, Metadata: kind.Metadata{UUID: house.UUID}}}

		left, err := f.dependents.Cascade(ctx, houseRecord, []graph.Dependent{{Root: web}}, 0)
		require.NoError(t, err)
		assert.Zero(t, left)
	})
}

func TestRecreated(t *testing.T) {
	t.Parallel()

	assert.False(t, dependents.Recreated(nil))
	assert.False(t, dependents.Recreated([]graph.Dependent{{Root: web}}), "one that only waits on it")
	assert.True(t, dependents.Recreated([]graph.Dependent{{Root: web}, {Root: api, Recreates: true}}))
}
