package cascade_test

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/khanzadimahdi/testproject/application/workload/controlplane/kinds/cascade"
	"github.com/khanzadimahdi/testproject/application/workload/controlplane/kinds/kindstest"
	"github.com/khanzadimahdi/testproject/domain"
	"github.com/khanzadimahdi/testproject/domain/workload/graph"
	"github.com/khanzadimahdi/testproject/domain/workload/graph/graphtest"
	"github.com/khanzadimahdi/testproject/domain/workload/kind"
	"github.com/khanzadimahdi/testproject/domain/workload/resource"
	resourcesMemory "github.com/khanzadimahdi/testproject/infrastructure/repository/memory/workload/resources"
)

// house is the house the fans in these tests are in, and elsewhere another.
var (
	house = kind.Reference{Kind: kindstest.Parent, UUID: kindstest.House}

	// restoredAt is when the house was restored: after its fans were last
	// seen.
	restoredAt = kindstest.Moment.Add(time.Hour)
	elsewhere  = func(f *kindstest.Fan) {
		f.Metadata.Refs = []kind.Reference{{Kind: kindstest.Parent, UUID: "house-2"}}
	}

	// keptBeforeRefs is a fan kept before refs: the house it is in is among
	// its owners.
	keptBeforeRefs = func(f *kindstest.Fan) {
		f.Metadata.Owners, f.Metadata.Refs = f.Metadata.Refs, nil
	}
)

// fans is a cascade over fans whose rules on their house are rules.
func fans(t *testing.T, rules kind.ParentRules, records ...resource.Record) (*cascade.Cascade, *resourcesMemory.Repository) {
	t.Helper()

	d := kindstest.Descriptor()
	d.OnParent = rules

	registry := kind.NewRegistry[kind.ControlPlaneBinding]()
	require.NoError(t, registry.Register(kind.BindControlPlane[kindstest.Spec, kindstest.Status](d, &kindstest.Fans{})))

	resources := resourcesMemory.NewRepository()
	for _, r := range records {
		_, err := resources.Create(context.Background(), r)
		require.NoError(t, err)
	}

	return cascade.New(registry, resources), resources
}

func held(t *testing.T, resources resource.Repository, uuid string) (resource.Record, bool) {
	t.Helper()

	r, err := resources.GetOne(context.Background(), kindstest.Kind, uuid)
	if errors.Is(err, domain.ErrNotExists) {
		return resource.Record{}, false
	}

	require.NoError(t, err)

	return r, true
}

func TestCascade_Deleted(t *testing.T) {
	t.Parallel()

	ctx := context.Background()

	t.Run("what goes with its parent goes when the parent is deleted, and nothing in another", func(t *testing.T) {
		t.Parallel()

		c, resources := fans(t, kind.ParentRules{Delete: kind.CascadeDelete, Restore: kind.CascadeKeep},
			kindstest.AFan("in-it", kindstest.Running, kindstest.Running),
			kindstest.AFan("also-in-it", kindstest.Stopped, kindstest.Stopped),
			kindstest.AFan("kept-in-it-before-refs", kindstest.Running, kindstest.Running, keptBeforeRefs),
			kindstest.AFan("in-another", kindstest.Running, kindstest.Running, elsewhere),
		)

		require.NoError(t, c.Deleted(ctx, house))

		_, kept := held(t, resources, "in-it")
		assert.False(t, kept)

		_, kept = held(t, resources, "also-in-it")
		assert.False(t, kept)

		_, kept = held(t, resources, "kept-in-it-before-refs")
		assert.False(t, kept, "its house among its owners")

		_, kept = held(t, resources, "in-another")
		assert.True(t, kept)
	})

	t.Run("what outlives its parent is kept as it is", func(t *testing.T) {
		t.Parallel()

		c, resources := fans(t, kind.ParentRules{Delete: kind.CascadeKeep, Restore: kind.CascadeKeep},
			kindstest.AFan("in-it", kindstest.Running, kindstest.Running),
		)

		require.NoError(t, c.Deleted(ctx, house))

		fan, kept := held(t, resources, "in-it")
		require.True(t, kept)
		assert.Equal(t, int64(1), fan.Version)
	})

	t.Run("nothing lives in a parent of another kind", func(t *testing.T) {
		t.Parallel()

		c, resources := fans(t, kind.ParentRules{Delete: kind.CascadeDelete, Restore: kind.CascadeDelete},
			kindstest.AFan("in-it", kindstest.Running, kindstest.Running),
		)

		require.NoError(t, c.Deleted(ctx, kind.Reference{Kind: "street", UUID: kindstest.House}))
		require.NoError(t, c.Restored(ctx, kind.Reference{Kind: "street", UUID: kindstest.House}, restoredAt))

		_, kept := held(t, resources, "in-it")
		assert.True(t, kept)
	})

	t.Run("what could not be read is said", func(t *testing.T) {
		t.Parallel()

		c, resources := fans(t, kind.ParentRules{Delete: kind.CascadeDelete, Restore: kind.CascadeKeep})
		resources.Fail = errors.New("the database is gone")

		assert.ErrorIs(t, c.Deleted(ctx, house), resources.Fail)
	})
}

func TestCascade_Restored(t *testing.T) {
	t.Parallel()

	ctx := context.Background()

	t.Run("what is reset to its parent is marked to be, and nothing in another", func(t *testing.T) {
		t.Parallel()

		c, resources := fans(t, kind.ParentRules{Delete: kind.CascadeDelete, Restore: kind.CascadeReset},
			kindstest.AFan("in-it", kindstest.Running, kindstest.Running),
			kindstest.AFan("kept-in-it-before-refs", kindstest.Running, kindstest.Running, keptBeforeRefs),
			kindstest.AFan("in-another", kindstest.Running, kindstest.Running, elsewhere),
		)

		require.NoError(t, c.Restored(ctx, house, restoredAt))

		fan, kept := held(t, resources, "in-it")
		require.True(t, kept, "it is kept until the parent is looked into")
		assert.True(t, fan.Reset)
		assert.Equal(t, kindstest.Running, kindstest.Typed(fan).Status.State, "and is as it was until then")
		assert.Equal(t, restoredAt, kindstest.Typed(fan).Status.ObservedAt, "what was seen of it before the restore is older than what is known of it")

		before, _ := held(t, resources, "kept-in-it-before-refs")
		assert.True(t, before.Reset, "its house among its owners")

		other, _ := held(t, resources, "in-another")
		assert.False(t, other.Reset)

		require.NoError(t, c.Restored(ctx, house, restoredAt))

		again, _ := held(t, resources, "in-it")
		assert.Equal(t, fan.Version, again.Version, "one marked already is not written again")
	})

	t.Run("what goes with its parent goes with a restore too, and what outlives it stays", func(t *testing.T) {
		t.Parallel()

		gone, goneResources := fans(t, kind.ParentRules{Delete: kind.CascadeDelete, Restore: kind.CascadeDelete},
			kindstest.AFan("in-it", kindstest.Running, kindstest.Running),
		)
		require.NoError(t, gone.Restored(ctx, house, restoredAt))

		_, kept := held(t, goneResources, "in-it")
		assert.False(t, kept)

		stays, staysResources := fans(t, kind.ParentRules{Delete: kind.CascadeDelete, Restore: kind.CascadeKeep},
			kindstest.AFan("in-it", kindstest.Running, kindstest.Running),
		)
		require.NoError(t, stays.Restored(ctx, house, restoredAt))

		fan, kept := held(t, staysResources, "in-it")
		require.True(t, kept)
		assert.False(t, fan.Reset)
	})

	t.Run("one written by something else in the meantime is marked all the same", func(t *testing.T) {
		t.Parallel()

		d := kindstest.Descriptor()
		d.OnParent = kind.ParentRules{Delete: kind.CascadeDelete, Restore: kind.CascadeReset}

		registry := kind.NewRegistry[kind.ControlPlaneBinding]()
		require.NoError(t, registry.Register(kind.BindControlPlane[kindstest.Spec, kindstest.Status](d, &kindstest.Fans{})))

		memory := resourcesMemory.NewRepository()
		_, err := memory.Create(ctx, kindstest.AFan("in-it", kindstest.Running, kindstest.Running))
		require.NoError(t, err)

		racing := &kindstest.Racing{Repository: memory}
		racing.Cross(kindstest.Rewrite(memory, func(r *resource.Record) { r.Attempts = 3 }))

		require.NoError(t, cascade.New(registry, racing).Restored(ctx, house, restoredAt))

		fan, _ := held(t, memory, "in-it")
		assert.True(t, fan.Reset)
		assert.Equal(t, 3, fan.Attempts, "on what the other wrote")
	})
}

func TestRepository_Delete(t *testing.T) {
	t.Parallel()

	ctx := context.Background()

	// repository is a cascading repository over fans in their house, the house
	// kept beside them as a record of its own.
	repository := func(t *testing.T, rules kind.ParentRules, records ...resource.Record) (*cascade.Repository, *resourcesMemory.Repository) {
		t.Helper()

		_, memory := fans(t, rules, records...)

		d := kindstest.Descriptor()
		d.OnParent = rules

		registry := kind.NewRegistry[kind.ControlPlaneBinding]()
		require.NoError(t, registry.Register(kind.BindControlPlane[kindstest.Spec, kindstest.Status](d, &kindstest.Fans{})))

		_, err := memory.Create(ctx, resource.Record{Raw: kind.Raw{Kind: kindstest.Parent, Metadata: kind.Metadata{UUID: kindstest.House}}})
		require.NoError(t, err)

		return cascade.NewRepository(registry, memory), memory
	}

	t.Run("deleting a parent anywhere takes what goes with it first", func(t *testing.T) {
		t.Parallel()

		cascading, memory := repository(t, kind.ParentRules{Delete: kind.CascadeDelete, Restore: kind.CascadeReset},
			kindstest.AFan("in-it", kindstest.Running, kindstest.Running),
			kindstest.AFan("in-another", kindstest.Running, kindstest.Running, elsewhere),
		)

		require.NoError(t, cascading.Delete(ctx, kindstest.Parent, kindstest.House))

		_, err := memory.GetOne(ctx, kindstest.Parent, kindstest.House)
		assert.ErrorIs(t, err, domain.ErrNotExists, "the parent is gone")

		_, kept := held(t, memory, "in-it")
		assert.False(t, kept, "and what lived in it with it")

		_, kept = held(t, memory, "in-another")
		assert.True(t, kept)
	})

	t.Run("and leaves what outlives it", func(t *testing.T) {
		t.Parallel()

		cascading, memory := repository(t, kind.ParentRules{Delete: kind.CascadeKeep, Restore: kind.CascadeKeep},
			kindstest.AFan("in-it", kindstest.Running, kindstest.Running),
		)

		require.NoError(t, cascading.Delete(ctx, kindstest.Parent, kindstest.House))

		_, kept := held(t, memory, "in-it")
		assert.True(t, kept)
	})

	t.Run("deleting what nothing lives in deletes it alone", func(t *testing.T) {
		t.Parallel()

		cascading, memory := repository(t, kind.ParentRules{Delete: kind.CascadeDelete, Restore: kind.CascadeReset},
			kindstest.AFan("in-it", kindstest.Running, kindstest.Running),
			kindstest.AFan("beside-it", kindstest.Running, kindstest.Running),
		)

		require.NoError(t, cascading.Delete(ctx, kindstest.Kind, "in-it"))

		_, kept := held(t, memory, "in-it")
		assert.False(t, kept)

		_, kept = held(t, memory, "beside-it")
		assert.True(t, kept)
	})

	t.Run("a parent whose children could not be read is kept, to be deleted again", func(t *testing.T) {
		t.Parallel()

		cascading, memory := repository(t, kind.ParentRules{Delete: kind.CascadeDelete, Restore: kind.CascadeReset})
		memory.Fail = errors.New("the database is gone")

		assert.ErrorIs(t, cascading.Delete(ctx, kindstest.Parent, kindstest.House), memory.Fail)

		memory.Fail = nil

		_, err := memory.GetOne(ctx, kindstest.Parent, kindstest.House)
		assert.NoError(t, err)
	})

	t.Run("its restores are carried over by the same rules", func(t *testing.T) {
		t.Parallel()

		cascading, memory := repository(t, kind.ParentRules{Delete: kind.CascadeDelete, Restore: kind.CascadeReset},
			kindstest.AFan("in-it", kindstest.Running, kindstest.Running),
		)

		require.NoError(t, cascading.Cascade().Restored(ctx, house, restoredAt))

		fan, kept := held(t, memory, "in-it")
		require.True(t, kept)
		assert.True(t, fan.Reset)
	})
}

// looking is told of what changed, as the graphs of roots are, and looks,
// each time, for the house the fans are in.
type looking struct {
	graphtest.Told

	resources resource.Repository

	lock         sync.Mutex
	houseThereAt []bool
}

func (l *looking) Changed(ctx context.Context, ref graph.Ref) error {
	_, err := l.resources.GetOne(ctx, kindstest.Parent, kindstest.House)

	l.lock.Lock()
	l.houseThereAt = append(l.houseThereAt, err == nil)
	l.lock.Unlock()

	return l.Told.Changed(ctx, ref)
}

func TestRepository_Delete_graphs(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	gone := kindstest.Moment.Add(time.Hour)

	// owned has a fan owned by what owners name.
	owned := func(owners ...kind.Reference) func(f *kindstest.Fan) {
		return func(f *kindstest.Fan) { f.Metadata.Owners = owners }
	}

	// busy has a fan that was at it: tried for, waiting on a command, and
	// reset by a restore.
	busy := func(r resource.Record) resource.Record {
		r.Pending = &resource.Pending{Action: "start", IDs: []string{"command-1"}, SentAt: kindstest.Moment}
		r.Attempts = 3
		r.TriedAt = kindstest.Moment
		r.Reset = true

		return r
	}

	// graphed is a cascading repository over fans in their house, the house
	// kept beside them as a record of its own and the fans being roots of
	// their own graphs, which are kept made, and told of what changed.
	graphed := func(t *testing.T, records ...resource.Record) (*cascade.Repository, *resourcesMemory.Repository, *looking) {
		t.Helper()

		memory := resourcesMemory.NewRepository()
		for _, r := range append([]resource.Record{{Raw: kind.Raw{Kind: kindstest.Parent, Metadata: kind.Metadata{UUID: kindstest.House}}}}, records...) {
			_, err := memory.Create(ctx, r)
			require.NoError(t, err)
		}

		d := kindstest.Descriptor()
		d.OnParent = kind.ParentRules{Delete: kind.CascadeDelete, Restore: kind.CascadeReset}

		registry := kind.NewRegistry[kind.ControlPlaneBinding]()
		require.NoError(t, registry.Register(kind.BindControlPlane[kindstest.Spec, kindstest.Status](d, &kindstest.Fans{})))

		told := &looking{resources: memory}

		return cascade.NewRepository(registry, memory, cascade.WithGraphs(graph.Roots{kindstest.Kind}, told), cascade.WithClock(func() time.Time { return gone })), memory, told
	}

	cascading, memory, told := graphed(t,
		busy(kindstest.AFan("root", kindstest.Running, kindstest.Running)),
		kindstest.AFan("owned", kindstest.Running, kindstest.Running, owned(kind.Reference{Kind: kindstest.Kind, UUID: "root"})),
		kindstest.AFan("owned-by-what-is-gone", kindstest.Running, kindstest.Running, owned(kind.Reference{Kind: kindstest.Kind, UUID: "gone"})),
		kindstest.AFan("root-asked-to-go", kindstest.Running, kind.Deleted, func(f *kindstest.Fan) { f.Metadata.DeletionRequestedAt = kindstest.Moment }),
		kindstest.AFan("owned-and-asked-to-go", kindstest.Deleting, kind.Deleted, owned(kind.Reference{Kind: kindstest.Kind, UUID: "root"})),
		kindstest.AFan("root-kept-before-refs", kindstest.Stopped, kindstest.Stopped, keptBeforeRefs),
		kindstest.AFan("in-another", kindstest.Running, kindstest.Running, elsewhere),
		kindstest.AFan("going-elsewhere", kindstest.Running, kind.Deleted, elsewhere, func(f *kindstest.Fan) { f.Metadata.DeletionRequestedAt = kindstest.Moment }),
		kindstest.AFan("owned-by-what-is-going", kindstest.Running, kindstest.Running, owned(kind.Reference{Kind: kindstest.Kind, UUID: "going-elsewhere"})),
	)

	require.NoError(t, cascading.Delete(ctx, kindstest.Parent, kindstest.House))

	_, err := memory.GetOne(ctx, kindstest.Parent, kindstest.House)
	require.ErrorIs(t, err, domain.ErrNotExists, "the house is gone")

	t.Run("a root that was not asked to go is kept, as gone with its house, for its graph to make again", func(t *testing.T) {
		t.Parallel()

		root, kept := held(t, memory, "root")
		require.True(t, kept)

		fan := kindstest.Typed(root)

		assert.Equal(t, kind.Missing, fan.Status.State, "its kind's missing state")
		assert.Equal(t, kindstest.Running, fan.Status.Expected, "what it is to be is as it was")
		assert.Equal(t, "it went with its house", fan.Status.Reason)
		assert.Equal(t, gone, fan.Status.Since)
		assert.Equal(t, gone, fan.Status.ObservedAt)
		assert.Zero(t, fan.Status.Speed, "and nothing its node said of it is said any more")
		assert.Empty(t, fan.Metadata.Node, "it is on no node, its house gone")
		assert.Equal(t, []kind.Reference{house}, fan.Metadata.Refs, "and it lives in its house still, which is made again under its uuid")
		assert.Nil(t, root.Pending, "nothing is waited on")
		assert.Zero(t, root.Attempts, "nor was it tried for yet")
		assert.False(t, root.Reset)

		before, kept := held(t, memory, "root-kept-before-refs")
		require.True(t, kept, "a root kept before refs, its house among its owners, which own nothing")
		assert.Equal(t, kind.Missing, kindstest.Typed(before).Status.State)
		assert.Equal(t, kindstest.Stopped, kindstest.Typed(before).Status.Expected)
	})

	t.Run("and so is what something that is there still owns", func(t *testing.T) {
		t.Parallel()

		fan, kept := held(t, memory, "owned")
		require.True(t, kept)
		assert.Equal(t, kind.Missing, kindstest.Typed(fan).Status.State)
	})

	t.Run("but what was asked to go goes, and what nothing there owns, or only what was asked to go", func(t *testing.T) {
		t.Parallel()

		for _, uuid := range []string{"root-asked-to-go", "owned-and-asked-to-go", "owned-by-what-is-gone", "owned-by-what-is-going"} {
			_, kept := held(t, memory, uuid)
			assert.False(t, kept, uuid)
		}

		for _, uuid := range []string{"in-another", "going-elsewhere"} {
			_, kept := held(t, memory, uuid)
			assert.True(t, kept, "and what is in another house is no house's but its own: %s", uuid)
		}
	})

	t.Run("the graphs are told of what was kept and what went, once the house is gone", func(t *testing.T) {
		t.Parallel()

		var changed []string
		for _, ref := range told.Changes() {
			changed = append(changed, ref.UUID)
		}

		assert.ElementsMatch(t, []string{"root", "owned", "owned-by-what-is-gone", "root-asked-to-go", "owned-and-asked-to-go", "root-kept-before-refs", "owned-by-what-is-going"}, changed)

		told.lock.Lock()
		defer told.lock.Unlock()

		assert.NotContains(t, told.houseThereAt, true, "never while the house was there")
	})
}

func TestRepository_Delete_graphsNotKept(t *testing.T) {
	t.Parallel()

	ctx := context.Background()

	t.Run("with no graphs kept made, everything that goes with its house goes", func(t *testing.T) {
		t.Parallel()

		_, memory := fans(t, kind.ParentRules{Delete: kind.CascadeDelete, Restore: kind.CascadeKeep},
			kindstest.AFan("root", kindstest.Running, kindstest.Running),
		)

		registry := kind.NewRegistry[kind.ControlPlaneBinding]()
		require.NoError(t, registry.Register(kind.BindControlPlane[kindstest.Spec, kindstest.Status](kindstest.Descriptor(), &kindstest.Fans{})))

		require.NoError(t, cascade.NewRepository(registry, memory).Delete(ctx, kindstest.Parent, kindstest.House))

		_, kept := held(t, memory, "root")
		assert.False(t, kept)
	})

	t.Run("and what cannot be told is said, once the house is gone", func(t *testing.T) {
		t.Parallel()

		memory := resourcesMemory.NewRepository()
		for _, r := range []resource.Record{{Raw: kind.Raw{Kind: kindstest.Parent, Metadata: kind.Metadata{UUID: kindstest.House}}}, kindstest.AFan("root", kindstest.Running, kindstest.Running)} {
			_, err := memory.Create(ctx, r)
			require.NoError(t, err)
		}

		registry := kind.NewRegistry[kind.ControlPlaneBinding]()
		require.NoError(t, registry.Register(kind.BindControlPlane[kindstest.Spec, kindstest.Status](kindstest.Descriptor(), &kindstest.Fans{})))

		told := &graphtest.Told{Failure: errors.New("the database is gone")}

		err := cascade.NewRepository(registry, memory, cascade.WithGraphs(graph.Roots{kindstest.Kind}, told)).Delete(ctx, kindstest.Parent, kindstest.House)
		assert.ErrorIs(t, err, told.Failure)

		_, err = memory.GetOne(ctx, kindstest.Parent, kindstest.House)
		assert.ErrorIs(t, err, domain.ErrNotExists)

		_, kept := held(t, memory, "root")
		assert.True(t, kept)
	})
}
