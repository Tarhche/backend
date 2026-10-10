package cascade_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/khanzadimahdi/testproject/application/workload/controlplane/kinds/cascade"
	"github.com/khanzadimahdi/testproject/application/workload/controlplane/kinds/kindstest"
	"github.com/khanzadimahdi/testproject/domain"
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
		f.Metadata.Owners = []kind.Reference{{Kind: kindstest.Parent, UUID: "house-2"}}
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
			kindstest.AFan("in-another", kindstest.Running, kindstest.Running, elsewhere),
		)

		require.NoError(t, c.Deleted(ctx, house))

		_, kept := held(t, resources, "in-it")
		assert.False(t, kept)

		_, kept = held(t, resources, "also-in-it")
		assert.False(t, kept)

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
			kindstest.AFan("in-another", kindstest.Running, kindstest.Running, elsewhere),
		)

		require.NoError(t, c.Restored(ctx, house, restoredAt))

		fan, kept := held(t, resources, "in-it")
		require.True(t, kept, "it is kept until the parent is looked into")
		assert.True(t, fan.Reset)
		assert.Equal(t, kindstest.Running, kindstest.Typed(fan).Status.State, "and is as it was until then")
		assert.Equal(t, restoredAt, kindstest.Typed(fan).Status.ObservedAt, "what was seen of it before the restore is older than what is known of it")

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
