package ownership_test

import (
	"context"
	"errors"
	"maps"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/khanzadimahdi/testproject/application/workload/controlplane/graph/ownership"
	"github.com/khanzadimahdi/testproject/application/workload/controlplane/kinds/cascade"
	"github.com/khanzadimahdi/testproject/application/workload/controlplane/kinds/kindstest"
	"github.com/khanzadimahdi/testproject/domain"
	"github.com/khanzadimahdi/testproject/domain/workload/graph"
	"github.com/khanzadimahdi/testproject/domain/workload/kind"
	containerKind "github.com/khanzadimahdi/testproject/domain/workload/kinds/container"
	networkKind "github.com/khanzadimahdi/testproject/domain/workload/kinds/network"
	stackKind "github.com/khanzadimahdi/testproject/domain/workload/kinds/stack"
	vmKind "github.com/khanzadimahdi/testproject/domain/workload/kinds/vm"
	volumeKind "github.com/khanzadimahdi/testproject/domain/workload/kinds/volume"
	"github.com/khanzadimahdi/testproject/domain/workload/resource"
	resourcesMemory "github.com/khanzadimahdi/testproject/infrastructure/repository/memory/workload/resources"
)

// the records the tests own and link: a stack, shop, of web and api in a
// Docker VM, on its network, web mounting a volume.
var (
	vm      = graph.Ref{Kind: vmKind.Name, UUID: "vm-uuid"}
	network = graph.Ref{Kind: networkKind.Name, UUID: "network-uuid"}
	volume  = graph.Ref{Kind: volumeKind.Name, UUID: "volume-uuid"}
	web     = graph.Ref{Kind: containerKind.Name, UUID: "web-uuid"}
	api     = graph.Ref{Kind: containerKind.Name, UUID: "api-uuid"}
	shop    = graph.Ref{Kind: stackKind.Name, UUID: "shop-uuid"}
)

// idle is a kind's control-plane strategy that is never asked anything: what
// owns what reads its kind's descriptor alone.
type idle[Spec, Status any] struct{}

func (idle[Spec, Status]) Admit(context.Context, kind.Resource[Spec, Status]) (kind.Resource[Spec, Status], domain.ValidationErrors, error) {
	return kind.Resource[Spec, Status]{}, nil, errors.New("an idle kind admits nothing")
}

func (idle[Spec, Status]) Reconcile(context.Context, kind.Resource[Spec, Status]) ([]kind.Intent, error) {
	return nil, nil
}

func (idle[Spec, Status]) Apply(context.Context, kind.Resource[Spec, Status], string, any) (kind.Resource[Spec, Status], domain.ValidationErrors, error) {
	return kind.Resource[Spec, Status]{}, nil, errors.New("an idle kind applies nothing")
}

// registry holds the kinds a graph is made of, as their descriptors say:
// a network, a volume, a container and a stack live in a VM, which lives in
// nothing.
func registry(t *testing.T) *kind.Registry[kind.ControlPlaneBinding] {
	t.Helper()

	r := kind.NewRegistry[kind.ControlPlaneBinding]()

	for _, binding := range []kind.ControlPlaneBinding{
		kind.BindControlPlane[vmKind.Spec, vmKind.Status](vmKind.Descriptor(), idle[vmKind.Spec, vmKind.Status]{}),
		kind.BindControlPlane[networkKind.Spec, networkKind.Status](networkKind.Descriptor(), idle[networkKind.Spec, networkKind.Status]{}),
		kind.BindControlPlane[volumeKind.Spec, volumeKind.Status](volumeKind.Descriptor(), idle[volumeKind.Spec, volumeKind.Status]{}),
		kind.BindControlPlane[containerKind.Spec, containerKind.Status](containerKind.Descriptor(), idle[containerKind.Spec, containerKind.Status]{}),
		kind.BindControlPlane[stackKind.Spec, stackKind.Status](stackKind.Descriptor(), idle[stackKind.Spec, stackKind.Status]{}),
	} {
		require.NoError(t, r.Register(binding))
	}

	return r
}

// record is the record ref names, using refs and owned by owners.
func record(ref graph.Ref, refs []graph.Ref, owners []graph.Ref) resource.Record {
	return resource.Record{Raw: kind.Raw{
		Kind: ref.Kind,
		Metadata: kind.Metadata{
			UUID:      ref.UUID,
			OwnerUUID: kindstest.OwnerUUID,
			Refs:      refs,
			Owners:    owners,
			CreatedAt: kindstest.Moment,
			UpdatedAt: kindstest.Moment,
		},
	}}
}

// in is a record in the VM, using what else uses names, owned by nothing.
func in(ref graph.Ref, uses ...graph.Ref) resource.Record {
	return record(ref, append([]graph.Ref{vm}, uses...), nil)
}

// owned is a record in the VM, owned by owners.
func owned(ref graph.Ref, owners ...graph.Ref) resource.Record {
	return record(ref, []graph.Ref{vm}, owners)
}

// fixture is what owns the records kept in memory, which something else may
// write just before each of the next writes (Racing), on a clock a test
// moves on.
type fixture struct {
	memory    *resourcesMemory.Repository
	racing    *kindstest.Racing
	clock     *kindstest.Clock
	ownership *ownership.Ownership
}

// newFixture keeps records, each at its first version.
func newFixture(t *testing.T, records ...resource.Record) *fixture {
	t.Helper()

	f := &fixture{memory: resourcesMemory.NewRepository(), clock: kindstest.NewClock()}
	f.racing = &kindstest.Racing{Repository: f.memory}
	f.ownership = ownership.New(registry(t), f.racing, f.clock.Now)

	for _, r := range records {
		_, err := f.memory.Create(context.Background(), r)
		require.NoError(t, err)
	}

	return f
}

// stored is the record ref names as it is kept, which is there.
func (f *fixture) stored(t *testing.T, ref graph.Ref) resource.Record {
	t.Helper()

	r, kept := f.memory.Stored(ref.Kind, ref.UUID)
	require.True(t, kept, "%s/%s is kept", ref.Kind, ref.UUID)

	return r
}

// labelled is a crossing write that labels the record it crosses, written
// over whatever version is kept.
func (f *fixture) labelled(key string) func(ctx context.Context, r resource.Record) {
	return kindstest.Rewrite(f.memory, func(r *resource.Record) {
		labels := maps.Clone(r.Metadata.Labels)
		if labels == nil {
			labels = make(map[string]string)
		}

		labels[key] = "yes"
		r.Metadata.Labels = labels
	})
}

// failing keeps records as the repository it wraps does, but fails to read
// or to write the one record it names.
type failing struct {
	resource.Repository

	uuid    string
	failure error
}

func (f *failing) GetOne(ctx context.Context, kindName string, uuid string) (resource.Record, error) {
	if uuid == f.uuid {
		return resource.Record{}, f.failure
	}

	return f.Repository.GetOne(ctx, kindName, uuid)
}

func (f *failing) Update(ctx context.Context, r resource.Record) (resource.Record, error) {
	if r.Metadata.UUID == f.uuid {
		return resource.Record{}, f.failure
	}

	return f.Repository.Update(ctx, r)
}

func TestOwnership_Owners(t *testing.T) {
	t.Parallel()

	ctx := context.Background()

	gone := graph.Ref{Kind: containerKind.Name, UUID: "gone-uuid"}

	for name, tt := range map[string]struct {
		records []resource.Record
		of      graph.Ref
		want    []graph.Ref
	}{
		"what owns it, each whose record is there, in the order of their kinds and their uuids": {
			records: []resource.Record{owned(network, shop, web, gone, api), in(shop), in(web), in(api)},
			of:      network,
			want:    []graph.Ref{api, web, shop},
		},
		"never the vm it lives in, which a record kept before refs has among its owners": {
			records: []resource.Record{record(network, nil, []graph.Ref{vm, shop}), record(vm, nil, nil), in(shop)},
			of:      network,
			want:    []graph.Ref{shop},
		},
		"each once, however often its record names it": {
			records: []resource.Record{owned(network, shop, shop), in(shop)},
			of:      network,
			want:    []graph.Ref{shop},
		},
		"and a vm, which lives in nothing, is owned by what owns it": {
			records: []resource.Record{record(vm, nil, []graph.Ref{web}), in(web)},
			of:      vm,
			want:    []graph.Ref{web},
		},
		"a record that is gone is owned by nothing": {
			records: []resource.Record{in(shop)},
			of:      network,
		},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			f := newFixture(t, tt.records...)

			owners, err := f.ownership.Owners(ctx, tt.of)
			require.NoError(t, err)

			assert.Equal(t, tt.want, owners)
		})
	}

	t.Run("a record of a kind not run here cannot be said to be owned", func(t *testing.T) {
		t.Parallel()

		fan := graph.Ref{Kind: kindstest.Kind, UUID: "fan-uuid"}

		f := newFixture(t, record(fan, nil, []graph.Ref{shop}), in(shop))

		_, err := f.ownership.Owners(ctx, fan)
		assert.ErrorIs(t, err, kind.ErrUnknownKind)
	})

	t.Run("and one whose owner cannot be looked for is an error naming it", func(t *testing.T) {
		t.Parallel()

		broken := errors.New("the database is gone")

		memory := resourcesMemory.NewRepository(owned(network, shop), in(shop))
		o := ownership.New(registry(t), &failing{Repository: memory, uuid: shop.UUID, failure: broken}, nil)

		_, err := o.Owners(ctx, network)
		require.ErrorIs(t, err, broken)
		assert.EqualError(t, err, "looking for stack/shop-uuid, which owns network/network-uuid: the database is gone")
	})
}

func TestOwnership_Disown(t *testing.T) {
	t.Parallel()

	ctx := context.Background()

	t.Run("owners are taken out of what owns it, which is what owns it still, as Owners says it", func(t *testing.T) {
		t.Parallel()

		// kept before refs: the vm it lives in among its owners.
		f := newFixture(t, record(network, nil, []graph.Ref{vm, shop, web, api}), record(vm, nil, nil), in(shop), in(web), in(api))
		f.clock.Advance(time.Minute)

		owners, err := f.ownership.Disown(ctx, network, web, api, volume)
		require.NoError(t, err)
		assert.Equal(t, []graph.Ref{shop}, owners, "what does not own it is taken out of nothing, and the vm owns nothing")

		stored := f.stored(t, network)
		assert.Equal(t, []kind.Reference{vm, shop}, stored.Metadata.Owners, "what its record says is otherwise as it was")
		assert.Equal(t, int64(2), stored.Version, "in one write")
		assert.Equal(t, f.clock.Now(), stored.Metadata.UpdatedAt)
	})

	t.Run("one owned by none of them is not written", func(t *testing.T) {
		t.Parallel()

		f := newFixture(t, owned(network, shop), in(shop))

		owners, err := f.ownership.Disown(ctx, network, web)
		require.NoError(t, err)
		assert.Equal(t, []graph.Ref{shop}, owners)
		assert.Equal(t, int64(1), f.stored(t, network).Version)
	})

	t.Run("one that is gone is disowned already, and owned by nothing", func(t *testing.T) {
		t.Parallel()

		f := newFixture(t, in(shop))

		owners, err := f.ownership.Disown(ctx, network, shop)
		require.NoError(t, err)
		assert.Empty(t, owners)
	})

	t.Run("what something else wrote in the meantime is read again, and kept", func(t *testing.T) {
		t.Parallel()

		f := newFixture(t, owned(network, shop, web), in(shop), in(web))
		f.racing.Cross(f.labelled("heard"))

		owners, err := f.ownership.Disown(ctx, network, web)
		require.NoError(t, err)
		assert.Equal(t, []graph.Ref{shop}, owners)

		stored := f.stored(t, network)
		assert.Equal(t, []kind.Reference{shop}, stored.Metadata.Owners)
		assert.Equal(t, "yes", stored.Metadata.Labels["heard"], "what crossed it is kept")
		assert.Equal(t, int64(3), stored.Version)
	})

	t.Run("three times at most", func(t *testing.T) {
		t.Parallel()

		f := newFixture(t, owned(network, shop, web), in(shop), in(web))
		f.racing.Cross(f.labelled("first"), f.labelled("second"), f.labelled("third"))

		_, err := f.ownership.Disown(ctx, network, web)
		require.ErrorIs(t, err, resource.ErrConflict)

		assert.Equal(t, []kind.Reference{shop, web}, f.stored(t, network).Metadata.Owners, "and it is owned as it was")
	})
}

func TestOwnership_Link(t *testing.T) {
	t.Parallel()

	ctx := context.Background()

	for name, tt := range map[string]struct {
		records    []resource.Record
		dependency graph.Ref
		dependent  graph.Ref
		edge       graph.EdgeType

		// owners are what owns the dependency once it is linked, and refs
		// what the dependent uses.
		owners []kind.Reference
		refs   []kind.Reference
	}{
		"an owned edge has the dependent own what it needs, and not use it": {
			records:    []resource.Record{in(network), in(web, network)},
			dependency: network, dependent: web, edge: graph.Owned,
			owners: []kind.Reference{web},
			refs:   []kind.Reference{vm},
		},
		"but the vm it lives in it uses, whatever owns what": {
			records:    []resource.Record{record(vm, nil, nil), in(web)},
			dependency: vm, dependent: web, edge: graph.Owned,
			owners: []kind.Reference{web},
			refs:   []kind.Reference{vm},
		},
		"beside what else owns it": {
			records:    []resource.Record{owned(network, shop), in(web)},
			dependency: network, dependent: web, edge: graph.Owned,
			owners: []kind.Reference{shop, web},
			refs:   []kind.Reference{vm},
		},
		"a loose edge has the dependent use what it needs, and not own it": {
			records:    []resource.Record{owned(volume, web, api), in(web)},
			dependency: volume, dependent: web, edge: graph.Loose,
			owners: []kind.Reference{api},
			refs:   []kind.Reference{vm, volume},
		},
		"and the vm it lives in, which it uses already, no longer owned by it": {
			records:    []resource.Record{record(vm, nil, []graph.Ref{web}), in(web)},
			dependency: vm, dependent: web, edge: graph.Loose,
			refs: []kind.Reference{vm},
		},
		"a stack's containers are what it owns": {
			records:    []resource.Record{in(web), in(shop)},
			dependency: web, dependent: shop, edge: graph.Owned,
			owners: []kind.Reference{shop},
			refs:   []kind.Reference{vm},
		},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			f := newFixture(t, tt.records...)

			require.NoError(t, f.ownership.Link(ctx, tt.dependency, tt.dependent, tt.edge))

			assert.Equal(t, tt.owners, f.stored(t, tt.dependency).Metadata.Owners, "what owns what it needs")
			assert.Equal(t, tt.refs, f.stored(t, tt.dependent).Metadata.Refs, "what the dependent uses")

			dependency, dependent := f.stored(t, tt.dependency).Version, f.stored(t, tt.dependent).Version

			require.NoError(t, f.ownership.Link(ctx, tt.dependency, tt.dependent, tt.edge))

			assert.Equal(t, dependency, f.stored(t, tt.dependency).Version, "linked again, nothing is written")
			assert.Equal(t, dependent, f.stored(t, tt.dependent).Version)
		})
	}

	t.Run("an edge that changes type is linked as it is now, back and forth", func(t *testing.T) {
		t.Parallel()

		f := newFixture(t, in(volume), in(web))

		for _, step := range []struct {
			edge   graph.EdgeType
			owners []kind.Reference
			refs   []kind.Reference
		}{
			{edge: graph.Loose, refs: []kind.Reference{vm, volume}},
			{edge: graph.Owned, owners: []kind.Reference{web}, refs: []kind.Reference{vm}},
			{edge: graph.Loose, refs: []kind.Reference{vm, volume}},
		} {
			require.NoError(t, f.ownership.Link(ctx, volume, web, step.edge))

			assert.Equal(t, step.owners, f.stored(t, volume).Metadata.Owners, "%s: what owns the volume", step.edge)
			assert.Equal(t, step.refs, f.stored(t, web).Metadata.Refs, "%s: what web uses", step.edge)
		}
	})

	t.Run("an order links nothing", func(t *testing.T) {
		t.Parallel()

		f := newFixture(t, in(api), in(web))

		require.NoError(t, f.ownership.Link(ctx, api, web, graph.Order))

		assert.Equal(t, int64(1), f.stored(t, api).Version)
		assert.Equal(t, int64(1), f.stored(t, web).Version)
	})

	t.Run("a record that is gone is linked to nothing, and the other is linked all the same", func(t *testing.T) {
		t.Parallel()

		f := newFixture(t, in(web, network), owned(volume, web))

		require.NoError(t, f.ownership.Link(ctx, network, web, graph.Owned), "the network is gone")
		assert.Equal(t, []kind.Reference{vm}, f.stored(t, web).Metadata.Refs, "what web owns it does not use")

		require.NoError(t, f.ownership.Link(ctx, volume, api, graph.Loose), "api is gone")
		assert.Equal(t, []kind.Reference{web}, f.stored(t, volume).Metadata.Owners, "what api does not own it does not own")

		require.NoError(t, f.ownership.Link(ctx, network, api, graph.Owned), "and nothing at all when both are")
	})

	t.Run("what something else wrote in the meantime is read again, and kept, on either end", func(t *testing.T) {
		t.Parallel()

		f := newFixture(t, in(network), in(web, network))

		// the network's write crossed, its write again not, and then web's
		// crossed.
		f.racing.Cross(f.labelled("heard"), func(context.Context, resource.Record) {}, f.labelled("heard"))

		require.NoError(t, f.ownership.Link(ctx, network, web, graph.Owned))

		n, w := f.stored(t, network), f.stored(t, web)

		assert.Equal(t, []kind.Reference{web}, n.Metadata.Owners)
		assert.Equal(t, "yes", n.Metadata.Labels["heard"])
		assert.Equal(t, []kind.Reference{vm}, w.Metadata.Refs)
		assert.Equal(t, "yes", w.Metadata.Labels["heard"])
		assert.Equal(t, f.clock.Now(), w.Metadata.UpdatedAt)
	})

	t.Run("three times at most, which is an error saying what it was writing", func(t *testing.T) {
		t.Parallel()

		f := newFixture(t, in(network), in(web, network))
		f.racing.Cross(f.labelled("first"), f.labelled("second"), f.labelled("third"))

		err := f.ownership.Link(ctx, network, web, graph.Owned)
		require.ErrorIs(t, err, resource.ErrConflict)
		assert.ErrorContains(t, err, "network/network-uuid cannot be written as owned by container/web-uuid")

		assert.Empty(t, f.stored(t, network).Metadata.Owners)
		assert.Equal(t, []kind.Reference{vm, network}, f.stored(t, web).Metadata.Refs, "and what it took away is taken away after what it gave")
	})

	t.Run("a loose edge that cannot be written says so too", func(t *testing.T) {
		t.Parallel()

		broken := errors.New("the database is gone")

		memory := resourcesMemory.NewRepository(owned(volume, web), in(web))
		o := ownership.New(registry(t), &failing{Repository: memory, uuid: volume.UUID, failure: broken}, nil)

		err := o.Link(ctx, volume, web, graph.Loose)
		require.ErrorIs(t, err, broken)
		assert.EqualError(t, err, "volume/volume-uuid cannot be written as not owned by container/web-uuid: the database is gone")

		r, _ := memory.Stored(web.Kind, web.UUID)
		assert.Equal(t, []kind.Reference{vm, volume}, r.Metadata.Refs, "what it gave is given")
	})

	t.Run("a dependent of a kind not run here cannot own, and an edge of no type is refused", func(t *testing.T) {
		t.Parallel()

		fan := graph.Ref{Kind: kindstest.Kind, UUID: "fan-uuid"}

		f := newFixture(t, in(network), record(fan, nil, nil))

		assert.ErrorIs(t, f.ownership.Link(ctx, network, fan, graph.Owned), kind.ErrUnknownKind)
		assert.Error(t, f.ownership.Link(ctx, network, web, graph.EdgeType(0)))
		assert.Equal(t, int64(1), f.stored(t, network).Version, "and nothing is written")
	})
}

func TestOwnership_Forget(t *testing.T) {
	t.Parallel()

	ctx := context.Background()

	t.Run("a root's record is removed, and one gone is forgotten already", func(t *testing.T) {
		t.Parallel()

		f := newFixture(t, in(shop), in(web))

		require.NoError(t, f.ownership.Forget(ctx, shop))

		_, kept := f.memory.Stored(shop.Kind, shop.UUID)
		assert.False(t, kept)

		_, kept = f.memory.Stored(web.Kind, web.UUID)
		assert.True(t, kept, "and nothing else")

		assert.NoError(t, f.ownership.Forget(ctx, shop))
	})

	t.Run("through the repository records are kept in, which carries it over to what lives in it", func(t *testing.T) {
		t.Parallel()

		kinds := registry(t)
		memory := resourcesMemory.NewRepository(record(vm, nil, nil), in(web), in(network))

		o := ownership.New(kinds, cascade.NewRepository(kinds, memory), nil)

		require.NoError(t, o.Forget(ctx, vm))

		for _, ref := range []graph.Ref{vm, web, network} {
			_, kept := memory.Stored(ref.Kind, ref.UUID)
			assert.False(t, kept, "%s/%s", ref.Kind, ref.UUID)
		}
	})
}
