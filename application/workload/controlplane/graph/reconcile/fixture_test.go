package reconcile_test

import (
	"context"
	"errors"
	"log/slog"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/khanzadimahdi/testproject/application/workload/controlplane/graph/execute"
	"github.com/khanzadimahdi/testproject/application/workload/controlplane/graph/reconcile"
	"github.com/khanzadimahdi/testproject/application/workload/controlplane/kinds/kindstest"
	"github.com/khanzadimahdi/testproject/domain"
	"github.com/khanzadimahdi/testproject/domain/workload/graph"
	"github.com/khanzadimahdi/testproject/domain/workload/graph/graphtest"
	"github.com/khanzadimahdi/testproject/domain/workload/kind"
	containerKind "github.com/khanzadimahdi/testproject/domain/workload/kinds/container"
	networkKind "github.com/khanzadimahdi/testproject/domain/workload/kinds/network"
	stackKind "github.com/khanzadimahdi/testproject/domain/workload/kinds/stack"
	vmKind "github.com/khanzadimahdi/testproject/domain/workload/kinds/vm"
	volumeKind "github.com/khanzadimahdi/testproject/domain/workload/kinds/volume"
	"github.com/khanzadimahdi/testproject/domain/workload/resource"
	executionsMemory "github.com/khanzadimahdi/testproject/infrastructure/repository/memory/workload/executions"
	leasesMemory "github.com/khanzadimahdi/testproject/infrastructure/repository/memory/workload/leases"
	resourcesMemory "github.com/khanzadimahdi/testproject/infrastructure/repository/memory/workload/resources"
)

// the nodes of the graph of the root the tests reconcile: a container, web,
// in its Docker VM, on its network and mounting its volume. A fake kind's
// node is named by its record's uuid, so a root's record, kept under web's
// name, is its root.
var (
	vm      = graph.ID(vmKind.Name, "v-uuid")
	network = graph.ID(networkKind.Name, "s_default")
	volume  = graph.ID(volumeKind.Name, "s_data")
	web     = graph.ID(containerKind.Name, "web")
)

// every is every kind a graph is made of here.
var every = []string{vmKind.Name, networkKind.Name, volumeKind.Name, containerKind.Name, stackKind.Name}

// settle is how long what a test waits for is given, and tick how often it
// is looked at meanwhile.
const (
	settle = 5 * time.Second
	tick   = time.Millisecond
)

// refOf is the record the node id names, as a fake kind says it is.
func refOf(id graph.NodeID) graph.Ref {
	return graphtest.Ref(id)
}

// requirement is the node id as a fake kind requires it, made after the
// nodes after names.
func requirement(id graph.NodeID, after ...graph.NodeID) graph.Requirement {
	r := graphtest.Requirement(id.Kind(), id.Name())
	r.After = after

	return r
}

// container is the root web, made after its vm, its network and its volume,
// which are made after the vm.
func container() *graphtest.Kind {
	kinds := graphtest.NewKind()
	kinds.Require(web, requirement(vm), requirement(network), requirement(volume))
	kinds.Require(network, requirement(vm))
	kinds.Require(volume, requirement(vm))

	return kinds
}

// idle is a kind's control-plane strategy that is never asked anything: the
// reconciler reads its kind's descriptor alone.
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

// registry holds the kinds a graph is made of, as their descriptors say: a
// network, a volume, a container and a stack live in a VM, which lives in
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

// fixture is reconcilers of roots whose graphs are made of one fake kind,
// which owns the records it holds as a test says, unless a test has the
// records say it; over records kept in memory, which every read and write of
// is counted; keeping how each node went in memory, every save recorded; and
// the leases in memory, on a clock a test moves on.
type fixture struct {
	kinds      *graphtest.Kind
	ownership  graph.Ownership
	registry   *kind.Registry[kind.ControlPlaneBinding]
	records    *resourcesMemory.Repository
	resources  *counting
	executions *recorder
	leases     *leasesMemory.Repository
	clock      *kindstest.Clock
}

func newFixture(t *testing.T, kinds *graphtest.Kind, records ...resource.Record) *fixture {
	t.Helper()

	f := &fixture{
		kinds:      kinds,
		ownership:  kinds.Ownership(),
		registry:   registry(t),
		records:    resourcesMemory.NewRepository(),
		executions: &recorder{Repository: executionsMemory.NewRepository()},
		clock:      kindstest.NewClock(),
	}

	f.resources = &counting{Repository: f.records}
	f.leases = leasesMemory.NewRepository(f.clock.Now)

	for _, r := range records {
		_, err := f.records.Create(context.Background(), r)
		require.NoError(t, err)
	}

	return f
}

// reconciler is a reconciler over the fixture of the kinds of root named,
// holding leases as holder, going about it as config says.
func (f *fixture) reconciler(t *testing.T, holder string, config reconcile.Config, roots ...string) *reconcile.Reconciler {
	t.Helper()

	return f.reconcilerOver(t, f.leases, holder, config, roots...)
}

// reconcilerOver is a reconciler over the fixture as reconciler is, keeping
// its leases in leases.
func (f *fixture) reconcilerOver(t *testing.T, leases graph.Leases, holder string, config reconcile.Config, roots ...string) *reconcile.Reconciler {
	t.Helper()

	kinds := f.kinds.Kinds(every...)

	executor, err := execute.New(kinds, f.ownership, f.executions, f.clock.Now)
	require.NoError(t, err)

	r, err := reconcile.New(reconcile.Dependencies{
		Registry:   f.registry,
		Resources:  f.resources,
		Kinds:      kinds,
		Roots:      roots,
		Planner:    graph.NewPlanner(kinds, graph.DefaultPolicy()),
		Executor:   executor,
		Executions: f.executions,
		Leases:     leases,
		Holder:     holder,
		Logger:     slog.New(slog.DiscardHandler),
	}, config)
	require.NoError(t, err)

	return r
}

// lease is the lease kept of the root the node id names, which is kept.
func (f *fixture) lease(t *testing.T, id graph.NodeID) graph.Lease {
	t.Helper()

	lease, kept := f.leases.Lease(id.Name())
	require.True(t, kept, "a lease of %s is kept", id)

	return lease
}

// lapse has the lease of the root the node id names lapse, its clock moved on
// a minute, and taken by holder, another control plane, as one would take it
// once it lapsed. Its holder's renewals run on a ticker of their own, which
// may renew it between the clock moving on and holder taking it: the clock
// is moved on again until holder takes it.
func (f *fixture) lapse(t *testing.T, id graph.NodeID, holder string) {
	t.Helper()

	require.Eventually(t, func() bool {
		f.clock.Advance(time.Minute)

		_, err := f.leases.Take(context.Background(), id.Name(), holder, time.Minute)

		return err == nil
	}, settle, tick, "%s never took the lease of %s once it lapsed", holder, id)
}

// kept is how each node of the root the node id names went, as it is kept.
func (f *fixture) kept(t *testing.T, id graph.NodeID) []graph.Execution {
	t.Helper()

	kept, err := f.executions.GetByRoot(context.Background(), id.Name())
	require.NoError(t, err)

	return kept
}

// record is the record ref names, of kindstest.OwnerUUID's, using refs and
// owned by owners, as changes say otherwise.
func record(ref graph.Ref, refs []graph.Ref, owners []graph.Ref, changes ...func(m *kind.Metadata)) resource.Record {
	r := resource.Record{Raw: kind.Raw{
		Kind: ref.Kind,
		Metadata: kind.Metadata{
			UUID:       ref.UUID,
			OwnerUUID:  kindstest.OwnerUUID,
			Refs:       refs,
			Owners:     owners,
			Generation: 1,
			CreatedAt:  kindstest.Moment,
			UpdatedAt:  kindstest.Moment,
		},
	}}

	for _, change := range changes {
		change(&r.Metadata)
	}

	return r
}

// in is the record ref names in the vm, owned by owners.
func in(ref graph.Ref, owners ...graph.Ref) resource.Record {
	return record(ref, []graph.Ref{refOf(vm)}, owners)
}

// root is the record of the root web, in the vm, owned by nothing, as
// changes say otherwise.
func root(changes ...func(m *kind.Metadata)) resource.Record {
	return record(refOf(web), []graph.Ref{refOf(vm)}, nil, changes...)
}

// deleting has a record asked to be deleted.
func deleting(m *kind.Metadata) {
	m.DeletionRequestedAt = kindstest.Moment
}

// counting keeps records as the repository it wraps does, counting every
// read, and recording every write, in order; a listing or a delete waiting
// on what a test stalls it on first.
type counting struct {
	resource.Repository

	lock   sync.Mutex
	reads  int
	writes []string

	// stalls are what each listing ("list") and each delete ("delete")
	// waits on before it is done, failing with what that fails with: none
	// until a test says.
	stalls map[string]func(ctx context.Context) error
}

// Stall has every listing ("list"), or every delete ("delete"), from now on
// wait on until before it is done, and fail with what until fails with.
func (c *counting) Stall(op string, until func(ctx context.Context) error) {
	c.lock.Lock()
	defer c.lock.Unlock()

	if c.stalls == nil {
		c.stalls = make(map[string]func(ctx context.Context) error)
	}

	c.stalls[op] = until
}

// stalled is what the listing or the delete op names waits on, and fails
// with: nothing when a test stalls it on nothing.
func (c *counting) stalled(ctx context.Context, op string) error {
	c.lock.Lock()
	until := c.stalls[op]
	c.lock.Unlock()

	if until == nil {
		return nil
	}

	return until(ctx)
}

func (c *counting) read() {
	c.lock.Lock()
	defer c.lock.Unlock()

	c.reads++
}

func (c *counting) wrote(write string, kindName string, uuid string) {
	c.lock.Lock()
	defer c.lock.Unlock()

	c.writes = append(c.writes, write+" "+kindName+"/"+uuid)
}

// Reads are how many reads there were.
func (c *counting) Reads() int {
	c.lock.Lock()
	defer c.lock.Unlock()

	return c.reads
}

// Writes are the writes there were, in order: what each did, and to what.
func (c *counting) Writes() []string {
	c.lock.Lock()
	defer c.lock.Unlock()

	return append([]string(nil), c.writes...)
}

func (c *counting) Create(ctx context.Context, r resource.Record) (resource.Record, error) {
	c.wrote("create", r.Kind, r.Metadata.UUID)

	return c.Repository.Create(ctx, r)
}

func (c *counting) Update(ctx context.Context, r resource.Record) (resource.Record, error) {
	c.wrote("update", r.Kind, r.Metadata.UUID)

	return c.Repository.Update(ctx, r)
}

func (c *counting) Delete(ctx context.Context, kindName string, uuid string) error {
	if err := c.stalled(ctx, "delete"); err != nil {
		return err
	}

	c.wrote("delete", kindName, uuid)

	return c.Repository.Delete(ctx, kindName, uuid)
}

func (c *counting) GetOne(ctx context.Context, kindName string, uuid string) (resource.Record, error) {
	c.read()

	return c.Repository.GetOne(ctx, kindName, uuid)
}

func (c *counting) GetOneByOwner(ctx context.Context, kindName string, ownerUUID string, uuid string) (resource.Record, error) {
	c.read()

	return c.Repository.GetOneByOwner(ctx, kindName, ownerUUID, uuid)
}

func (c *counting) GetOneBySlug(ctx context.Context, kindName string, slug string) (resource.Record, error) {
	c.read()

	return c.Repository.GetOneBySlug(ctx, kindName, slug)
}

func (c *counting) GetAll(ctx context.Context, kindName string, filter resource.Filter, offset uint, limit uint) ([]resource.Record, uint, error) {
	if err := c.stalled(ctx, "list"); err != nil {
		return nil, 0, err
	}

	c.read()

	return c.Repository.GetAll(ctx, kindName, filter, offset, limit)
}

// recorder keeps executions in memory, and every one it is handed to save,
// in order, unless a test has it fail to.
type recorder struct {
	*executionsMemory.Repository

	lock  sync.Mutex
	saved []graph.Execution
	fails error
}

func (r *recorder) Save(ctx context.Context, e graph.Execution) error {
	r.lock.Lock()
	fails := r.fails
	if fails == nil {
		r.saved = append(r.saved, e)
	}
	r.lock.Unlock()

	if fails != nil {
		return fails
	}

	return r.Repository.Save(ctx, e)
}

// Fail has every save from now on fail with err, keeping nothing.
func (r *recorder) Fail(err error) {
	r.lock.Lock()
	defer r.lock.Unlock()

	r.fails = err
}

// Saved are the executions saved, in the order they were.
func (r *recorder) Saved() []graph.Execution {
	r.lock.Lock()
	defer r.lock.Unlock()

	return append([]graph.Execution(nil), r.saved...)
}

// together keeps leases as the leases it wraps do, but answers none of the
// first who take one (Take), as many as it waits for, until each of them has,
// each with what was there when it took it: what takers who come to one at
// once find. Any who take one after them are answered at once.
type together struct {
	graph.Leases

	left   atomic.Int64
	takers sync.WaitGroup
}

// newTogether is leases whose first takes are answered once takers take
// them.
func newTogether(leases graph.Leases, takers int) *together {
	t := &together{Leases: leases}
	t.left.Store(int64(takers))
	t.takers.Add(takers)

	return t
}

func (t *together) Take(ctx context.Context, rootUUID string, holder string, ttl time.Duration) (graph.Lease, error) {
	lease, err := t.Leases.Take(ctx, rootUUID, holder, ttl)

	if t.left.Add(-1) >= 0 {
		t.takers.Done()
		t.takers.Wait()
	}

	return lease, err
}

// gate is what a fake kind's node waits on when it is asked something, until
// a test lets it go on, or its run is cancelled, which it fails with.
type gate struct {
	open chan struct{}
	once sync.Once
}

func newGate() *gate {
	return &gate{open: make(chan struct{})}
}

// wait waits for the gate to open, or for ctx to be done.
func (g *gate) wait(ctx context.Context) error {
	select {
	case <-g.open:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

// Open lets whatever waits on the gate, and will, go on.
func (g *gate) Open() {
	g.once.Do(func() { close(g.open) })
}
