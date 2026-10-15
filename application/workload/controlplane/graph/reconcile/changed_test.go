package reconcile_test

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/khanzadimahdi/testproject/application/workload/controlplane/graph/reconcile"
	"github.com/khanzadimahdi/testproject/domain/workload/graph"
	"github.com/khanzadimahdi/testproject/domain/workload/graph/graphtest"
	"github.com/khanzadimahdi/testproject/domain/workload/kind"
	containerKind "github.com/khanzadimahdi/testproject/domain/workload/kinds/container"
	networkKind "github.com/khanzadimahdi/testproject/domain/workload/kinds/network"
	stackKind "github.com/khanzadimahdi/testproject/domain/workload/kinds/stack"
	taskKind "github.com/khanzadimahdi/testproject/domain/workload/kinds/task"
	vmKind "github.com/khanzadimahdi/testproject/domain/workload/kinds/vm"
	volumeKind "github.com/khanzadimahdi/testproject/domain/workload/kinds/volume"
	"github.com/khanzadimahdi/testproject/domain/workload/resource"
)

// the records of a workload whose changes reach its roots: in one vm, two
// containers nobody owns, one using a volume, on a network both own and a
// network one of them owns; a stack, its service, and the network both own;
// and a container using a network that is gone. In another vm, a container
// nobody owns, and the network it owns.
var (
	theVM    = graph.Ref{Kind: vmKind.Name, UUID: "v-uuid"}
	otherVM  = graph.Ref{Kind: vmKind.Name, UUID: "w-uuid"}
	c1       = graph.Ref{Kind: containerKind.Name, UUID: "c1"}
	c2       = graph.Ref{Kind: containerKind.Name, UUID: "c2"}
	shared   = graph.Ref{Kind: networkKind.Name, UUID: "shared"}
	own      = graph.Ref{Kind: networkKind.Name, UUID: "own"}
	data     = graph.Ref{Kind: volumeKind.Name, UUID: "data"}
	shop     = graph.Ref{Kind: stackKind.Name, UUID: "shop"}
	service  = graph.Ref{Kind: containerKind.Name, UUID: "shop-web"}
	shopNet  = graph.Ref{Kind: networkKind.Name, UUID: "shop-default"}
	stranded = graph.Ref{Kind: containerKind.Name, UUID: "stranded"}
	goneNet  = graph.Ref{Kind: networkKind.Name, UUID: "gone-net"}
	c3       = graph.Ref{Kind: containerKind.Name, UUID: "c3"}
	theirs   = graph.Ref{Kind: networkKind.Name, UUID: "theirs"}
)

// workload is the records of that workload.
func workload() []resource.Record {
	return []resource.Record{
		record(theVM, nil, nil),
		record(c1, []graph.Ref{theVM, data}, nil),
		record(c2, []graph.Ref{theVM}, nil),
		record(shared, []graph.Ref{theVM}, []graph.Ref{c1, c2}),
		record(own, []graph.Ref{theVM}, []graph.Ref{c2}),
		record(data, []graph.Ref{theVM}, nil),
		record(shop, []graph.Ref{theVM}, nil),
		record(service, []graph.Ref{theVM}, []graph.Ref{shop}),
		record(shopNet, []graph.Ref{theVM}, []graph.Ref{shop, service}),
		record(stranded, []graph.Ref{theVM, goneNet}, nil),
		record(otherVM, nil, nil),
		record(c3, []graph.Ref{otherVM}, nil),
		record(theirs, []graph.Ref{otherVM}, []graph.Ref{c3}),
	}
}

func TestReconciler_Changed(t *testing.T) {
	t.Parallel()

	ctx := context.Background()

	for name, tt := range map[string]struct {
		changed graph.Ref
		roots   []string
		want    []graph.Ref
	}{
		"a network two roots own reaches both":                         {changed: shared, want: []graph.Ref{c1, c2}},
		"a network one root owns reaches it alone":                     {changed: own, want: []graph.Ref{c2}},
		"a volume a root uses reaches it":                              {changed: data, want: []graph.Ref{c1}},
		"a root reaches itself":                                        {changed: c1, want: []graph.Ref{c1}},
		"a stack's service reaches the stack, through what owns it":    {changed: service, want: []graph.Ref{shop}},
		"and so does the network the stack and its service own":        {changed: shopNet, want: []graph.Ref{shop}},
		"but not when stacks are not roots here":                       {changed: service, roots: []string{containerKind.Name}},
		"a network that is gone reaches what uses it, as its refs say": {changed: goneNet, want: []graph.Ref{stranded}},
		"a root that is gone reaches itself, to forget its lease":      {changed: graph.Ref{Kind: containerKind.Name, UUID: "gone"}, want: []graph.Ref{{Kind: containerKind.Name, UUID: "gone"}}},
		"a vm reaches every root in it, and nothing in another": {
			changed: theVM,
			want:    []graph.Ref{c1, c2, shop, stranded},
		},
		"another vm reaches its own":                 {changed: otherVM, want: []graph.Ref{c3}},
		"what is of no graph's kind reaches nothing": {changed: graph.Ref{Kind: taskKind.Name, UUID: "t-uuid"}},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			roots := tt.roots
			if roots == nil {
				roots = []string{containerKind.Name, stackKind.Name}
			}

			f := newFixture(t, container(), workload()...)
			r := f.reconciler(t, "one", reconcile.DefaultConfig(), roots...)

			require.NoError(t, r.Changed(ctx, tt.changed))

			assert.ElementsMatch(t, tt.want, r.Queued())
			assert.Empty(t, f.kinds.Calls(), "and nothing is reconciled yet")
			assert.Empty(t, f.resources.Writes(), "nor written")
		})
	}

	t.Run("with no kinds of root, nothing is read, and nothing reached", func(t *testing.T) {
		t.Parallel()

		f := newFixture(t, container(), workload()...)
		r := f.reconciler(t, "one", reconcile.DefaultConfig())

		require.NoError(t, r.Changed(ctx, theVM))

		assert.Empty(t, r.Queued())
		assert.Zero(t, f.resources.Reads())
	})

	t.Run("what cannot be read is what it fails with", func(t *testing.T) {
		t.Parallel()

		broken := errors.New("broken")

		f := newFixture(t, container(), workload()...)
		f.records.Fail = broken

		r := f.reconciler(t, "one", reconcile.DefaultConfig(), containerKind.Name)

		assert.ErrorIs(t, r.Changed(ctx, shared), broken)
	})
}

func TestReconciler_Dependents(t *testing.T) {
	t.Parallel()

	ctx := context.Background()

	var (
		theVM  = refOf(vm)
		theNet = refOf(network)
		theVol = refOf(volume)
		root   = refOf(web)
		other  = graph.Ref{Kind: containerKind.Name, UUID: "other"}
		going  = graph.Ref{Kind: containerKind.Name, UUID: "going"}
		stale  = graph.Ref{Kind: containerKind.Name, UUID: "stale"}
	)

	// in the vm, the root web, which owns its network and uses its volume;
	// other, a root whose graph needs the vm alone; going, a root asked to
	// go; and stale, a root whose record still says it uses the volume, which
	// its plan does not name.
	records := func() []resource.Record {
		named := func(m *kind.Metadata) { m.Name = m.UUID }

		return []resource.Record{
			record(theVM, nil, nil),
			record(root, []graph.Ref{theVM, theVol}, nil, named),
			record(theNet, []graph.Ref{theVM}, []graph.Ref{root}),
			record(theVol, []graph.Ref{theVM}, nil),
			record(other, []graph.Ref{theVM}, nil, named),
			record(going, []graph.Ref{theVM}, nil, named, deleting),
			record(stale, []graph.Ref{theVM, theVol}, nil, named),
		}
	}

	// kinds is web's graph, its volume external when it is not its own, and
	// other's and going's, which need the vm alone, every node there.
	kinds := func(external bool) *graphtest.Kind {
		k := container()

		vol := requirement(volume)
		vol.External = external

		k.Require(web, requirement(vm), requirement(network), vol)
		k.Require(graph.ID(containerKind.Name, "other"), requirement(vm))
		k.Require(graph.ID(containerKind.Name, "going"), requirement(vm))
		k.Hold(vm, network, volume)

		return k
	}

	for name, tt := range map[string]struct {
		record   graph.Ref
		external bool
		want     []graph.Dependent
	}{
		"a vm is needed by every live root whose plan has it, which makes it again": {
			record: theVM,
			want: []graph.Dependent{
				{Root: other, Name: "other", Recreates: true},
				{Root: root, Name: "web", Recreates: true},
			},
		},
		"a network a root owns is needed by it, which makes it again": {
			record: theNet,
			want:   []graph.Dependent{{Root: root, Name: "web", Recreates: true}},
		},
		"a volume a root uses is needed by it, and not by a root whose plan no longer names it": {
			record: theVol,
			want:   []graph.Dependent{{Root: root, Name: "web", Recreates: true}},
		},
		"a volume external to a root is needed by it, which only waits on it": {
			record:   theVol,
			external: true,
			want:     []graph.Dependent{{Root: root, Name: "web", Recreates: false}},
		},
		"a root is not its own dependent": {record: root},
		"what is of no graph's kind is needed by nothing": {
			record: graph.Ref{Kind: taskKind.Name, UUID: "t-uuid"},
		},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			f := newFixture(t, kinds(tt.external), records()...)
			r := f.reconciler(t, "one", reconcile.DefaultConfig(), containerKind.Name, stackKind.Name)

			dependents, err := r.Dependents(ctx, tt.record)
			require.NoError(t, err)

			assert.Equal(t, tt.want, dependents)
			assert.Empty(t, r.Queued(), "nothing is queued")
			assert.Empty(t, f.resources.Writes(), "nor written")
			assert.Empty(t, f.kinds.Asked(graphtest.Create), "nor made")
		})
	}

	t.Run("with no kinds of root, nothing is read, and nothing needs anything", func(t *testing.T) {
		t.Parallel()

		f := newFixture(t, kinds(false), records()...)
		r := f.reconciler(t, "one", reconcile.DefaultConfig())

		dependents, err := r.Dependents(ctx, theVM)
		require.NoError(t, err)

		assert.Empty(t, dependents)
		assert.Zero(t, f.resources.Reads())
	})

	t.Run("what a root needs that cannot be read is what it fails with", func(t *testing.T) {
		t.Parallel()

		broken := errors.New("broken")

		f := newFixture(t, kinds(false), records()...)
		f.kinds.On(graphtest.Root, web, func(context.Context) error { return broken })

		r := f.reconciler(t, "one", reconcile.DefaultConfig(), containerKind.Name)

		_, err := r.Dependents(ctx, theVM)
		assert.ErrorIs(t, err, broken)
	})

	t.Run("and so is a record that cannot be read", func(t *testing.T) {
		t.Parallel()

		broken := errors.New("broken")

		f := newFixture(t, kinds(false), records()...)
		f.records.Fail = broken

		r := f.reconciler(t, "one", reconcile.DefaultConfig(), containerKind.Name)

		_, err := r.Dependents(ctx, theVM)
		assert.ErrorIs(t, err, broken)
	})
}
