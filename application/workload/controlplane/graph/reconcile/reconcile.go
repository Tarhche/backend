// Package reconcile keeps every root's graph made for as long as the root
// lives: a stack, or a container no stack owns (graph.RootKind), whose graph
// is planned from its record and carried out again whenever what it needs
// changed, and every so often besides.
//
// A root is reconciled (Reconcile) under its lease (graph.Leases), which one
// control plane holds at a time, renewed while the root's plan runs, so that
// two never carry one root's plan out at once: a root another holds is
// skipped, and so is one backed off after its plan failed, until its backoff
// is over. A root asked to be deleted is removed, from the top down, through
// what it owns (execute.Executor.Remove), however often it is reconciled, and
// is never made again; any other is applied at its record's generation
// (execute.Executor.Apply), which makes what is missing of it, from the
// bottom up, and nothing else. How its plan ended is kept on its lease as it
// is let go of: one that failed backs the root off, longer with every failure
// in a row, one done clears that, and one that waits on what it needs leaves
// it. A root that is gone has its lease and how its plan went forgotten.
//
// What is reconciled is queued (Enqueue), each root once however often it is
// queued before it is reconciled, and reconciled by as many workers as the
// reconciler is given (Work), a root by one of them at a time. A record that
// changed is told to the reconciler (Changed, its graph.Notifier), which
// queues the roots it reaches through what owns it and what uses it, and
// what owns and uses those; and every so often every root is queued (Resync),
// which catches up with whatever no change was told of, and removes what
// records owned that are all gone, the end of a delete cut short after its
// root went.
//
// It runs the kinds it is given as roots, and no others: given none, nothing
// is queued, read or reconciled.
package reconcile

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"maps"
	"slices"
	"sync"
	"time"

	"github.com/khanzadimahdi/testproject/application/workload/controlplane/graph/execute"
	"github.com/khanzadimahdi/testproject/application/workload/controlplane/kinds/reconcileResources"
	"github.com/khanzadimahdi/testproject/domain"
	"github.com/khanzadimahdi/testproject/domain/workload/graph"
	"github.com/khanzadimahdi/testproject/domain/workload/kind"
	"github.com/khanzadimahdi/testproject/domain/workload/resource"
)

// ErrReconciling is a root reconciled while this reconciler is reconciling
// it already: a lease is taken again by the control plane that holds it, so
// it is the reconciler that keeps two of its own runs off one root.
var ErrReconciling = errors.New("the root is being reconciled here already")

// Config is how the reconciler goes about it.
type Config struct {
	// Workers are how many roots are reconciled at once, each by one of
	// them.
	Workers int

	// Lease is how long a root's lease is held before another control plane
	// may take it: it is renewed every third of it while the root's plan
	// runs, so that it lapses only once this control plane stops.
	Lease time.Duration

	// Backoff is how long a root whose plan failed is left before it is
	// reconciled again, doubling with every failure in a row.
	Backoff graph.Backoff

	// Batch is how many records a resync reads at a time.
	Batch uint
}

// DefaultConfig is how the control plane goes about it: the reconcile loop's
// backoff, 15 s doubling up to 15 min, among it.
func DefaultConfig() Config {
	loop := reconcileResources.DefaultConfig()

	return Config{
		Workers: 4,
		Lease:   30 * time.Second,
		Backoff: graph.Backoff{First: loop.Backoff, Most: loop.MaxBackoff},
		Batch:   100,
	}
}

// Dependencies are what the reconciler reads, plans and carries roots out
// with.
type Dependencies struct {
	// Registry is every kind the control plane runs, which says what each
	// lives in, and Resources the records of every kind.
	Registry  *kind.Registry[kind.ControlPlaneBinding]
	Resources resource.Repository

	// Kinds are the kinds a graph is made of, by their names, and Roots the
	// names of those among them whose resources are roots of their own when
	// nothing owns them: each a graph.RootKind.
	Kinds graph.Kinds
	Roots []string

	// Planner plans a root's graph, and Executor carries it out, keeping how
	// each node went in Executions.
	Planner    *graph.Planner
	Executor   *execute.Executor
	Executions graph.Executions

	// Leases keep which control plane carries each root out, and Holder is
	// this one, as the leases it takes say it.
	Leases graph.Leases
	Holder string

	Logger *slog.Logger
}

// Reconciler keeps every root's graph made, for as long as the root lives.
type Reconciler struct {
	registry   *kind.Registry[kind.ControlPlaneBinding]
	resources  resource.Repository
	kinds      graph.Kinds
	roots      map[string]graph.RootKind
	planner    *graph.Planner
	executor   *execute.Executor
	executions graph.Executions
	leases     graph.Leases
	holder     string
	logger     *slog.Logger
	config     Config

	queue *queue

	// reconciling are the roots being reconciled here.
	lock        sync.Mutex
	reconciling map[graph.Ref]bool
}

var _ graph.Notifier = &Reconciler{}

// New is a reconciler of the roots of the kinds dependencies names as roots,
// as config says. One of them that is not a kind of a graph, a kind whose
// resources can be roots (graph.RootKind), or a kind the control plane runs,
// is refused, and so is a kind of a graph the control plane does not run, a
// dependency missing, and a config that cannot be gone about: no worker, a
// lease shorter than three milliseconds, a backoff of less than nothing or a
// batch of nothing.
func New(dependencies Dependencies, config Config) (*Reconciler, error) {
	switch {
	case dependencies.Registry == nil, dependencies.Resources == nil, dependencies.Planner == nil, dependencies.Executor == nil,
		dependencies.Executions == nil, dependencies.Leases == nil, dependencies.Logger == nil:
		return nil, errors.New("a reconciler reads, plans, carries out and keeps through what it is given, and is given nothing of one of them")
	case len(dependencies.Holder) == 0:
		return nil, errors.New("a reconciler holds leases as a control plane, and is given none to hold them as")
	case config.Workers < 1:
		return nil, fmt.Errorf("a reconciler reconciles one root at a time at least, not %d", config.Workers)
	case config.Lease < 3*time.Millisecond:
		return nil, fmt.Errorf("a root's lease is renewed every third of it, and lasts three milliseconds at least, not %s", config.Lease)
	case config.Backoff.First < 0 || config.Backoff.Most < 0:
		return nil, fmt.Errorf("a backoff is no less than nothing, not %s up to %s", config.Backoff.First, config.Backoff.Most)
	case config.Batch < 1:
		return nil, errors.New("a resync reads a record at a time at least")
	}

	for _, kindName := range slices.Sorted(maps.Keys(dependencies.Kinds)) {
		if _, registered := dependencies.Registry.Lookup(kindName); !registered {
			return nil, fmt.Errorf("%w: %q, a kind of a graph, is not a kind the control plane runs", kind.ErrUnknownKind, kindName)
		}
	}

	roots := make(map[string]graph.RootKind, len(dependencies.Roots))

	for _, kindName := range dependencies.Roots {
		k, known := dependencies.Kinds[kindName]
		if !known {
			return nil, fmt.Errorf("%w: %q, a kind of root, is no kind of a graph", kind.ErrUnknownKind, kindName)
		}

		rooted, isRoot := k.(graph.RootKind)
		if !isRoot {
			return nil, fmt.Errorf("%q is no kind whose resources are roots of their own: it does not say what a root of it needs", kindName)
		}

		roots[kindName] = rooted
	}

	return &Reconciler{
		registry:    dependencies.Registry,
		resources:   dependencies.Resources,
		kinds:       dependencies.Kinds,
		roots:       roots,
		planner:     dependencies.Planner,
		executor:    dependencies.Executor,
		executions:  dependencies.Executions,
		leases:      dependencies.Leases,
		holder:      dependencies.Holder,
		logger:      dependencies.Logger,
		config:      config,
		queue:       newQueue(),
		reconciling: make(map[graph.Ref]bool),
	}, nil
}

// Reconcile reconciles root, under its lease, and is what that came to.
//
// A root being reconciled here already is skipped, ErrReconciling. Its lease
// is taken first, as this control plane's: a root another holds is skipped,
// ErrHeld; and so is one backed off, ErrBackedOff, unless it is gone
// meanwhile, when its lease is forgotten, since nothing would take it again
// to forget it. Then its record is read. One that is gone, or that something
// owns now and is a root no more, has its lease and how its plan went
// forgotten, and that is all. Any other's graph is planned, from what its
// kind says it needs (graph.RootKind.Root), and carried out while its lease
// is renewed every third of its time: removed when it was asked to be
// deleted, a delete cut short taken up where it stopped, and applied at its
// record's generation otherwise, a record kept before generations at its
// first; and a lease lost meanwhile, which another control plane took once
// it lapsed, stops the run.
//
// How it ended is kept on its lease as it is let go of: done, waiting, or
// failed, which a plan that could not be made is, and a run that was
// interrupted too, saying why; and a root removed has its lease forgotten. It
// is nothing when its plan is done, waits, or it is gone, and what failed
// otherwise, which is logged with its kind and its uuid, and how long it is
// backed off for. A root of a kind that is not a root here is
// kind.ErrUnknownKind, and nothing is done.
func (r *Reconciler) Reconcile(ctx context.Context, root graph.Ref) error {
	rooted, isRoot := r.roots[root.Kind]
	if !isRoot {
		return fmt.Errorf("%w: %q is no kind of root here", kind.ErrUnknownKind, root.Kind)
	}

	if !r.begin(root) {
		r.logger.DebugContext(ctx, "a root is being reconciled here already", "kind", root.Kind, "uuid", root.UUID)

		return ErrReconciling
	}
	defer r.end(root)

	lease, err := r.leases.Take(ctx, root.UUID, r.holder, r.config.Lease)
	switch {
	case errors.Is(err, graph.ErrBackedOff):
		return r.backedOff(ctx, root, lease, err)
	case errors.Is(err, graph.ErrHeld):
		r.logger.DebugContext(ctx, "a root is carried out by another control plane", "kind", root.Kind, "uuid", root.UUID, "holder", lease.Holder)

		return err
	case err != nil:
		r.logger.ErrorContext(ctx, "could not take a root's lease", "error", err, "kind", root.Kind, "uuid", root.UUID)

		return err
	}

	ended, gone, err := r.carry(ctx, root, rooted)
	if gone {
		return r.forget(ctx, root)
	}

	released, releaseErr := r.leases.Release(context.WithoutCancel(ctx), root.UUID, r.holder, ended)

	switch {
	case errors.Is(releaseErr, graph.ErrLost):
		r.logger.WarnContext(ctx, "a root's lease was lost before how its plan ended could be kept", "kind", root.Kind, "uuid", root.UUID, "state", ended.State)
	case releaseErr != nil:
		r.logger.ErrorContext(ctx, "could not let go of a root's lease", "error", releaseErr, "kind", root.Kind, "uuid", root.UUID, "state", ended.State)
	}

	switch {
	case err != nil && releaseErr == nil:
		r.logger.ErrorContext(ctx, "a root's plan failed, and it is backed off", "error", err, "kind", root.Kind, "uuid", root.UUID, "failures", released.Failures, "next_at", released.NextAt)
	case err != nil:
		r.logger.ErrorContext(ctx, "a root's plan failed", "error", err, "kind", root.Kind, "uuid", root.UUID)
	}

	return errors.Join(err, releaseErr)
}

// begin has root reconciled here from now, and reports whether it was not
// already.
func (r *Reconciler) begin(root graph.Ref) bool {
	r.lock.Lock()
	defer r.lock.Unlock()

	if r.reconciling[root] {
		return false
	}

	r.reconciling[root] = true

	return true
}

// end has root reconciled here no more.
func (r *Reconciler) end(root graph.Ref) {
	r.lock.Lock()
	defer r.lock.Unlock()

	delete(r.reconciling, root)
}

// carry carries out the plan of root, whose lease is taken, and is how it
// ended, as its lease keeps it, or whether its lease is to be forgotten
// instead, and what failed.
func (r *Reconciler) carry(ctx context.Context, root graph.Ref, rooted graph.RootKind) (graph.Ended, bool, error) {
	rec, there, err := r.read(ctx, root)
	switch {
	case err != nil:
		return r.failed(err)
	case !there:
		r.logger.InfoContext(ctx, "forgetting a root that is gone, its lease and how its plan went", "kind", root.Kind, "uuid", root.UUID)

		return graph.Ended{}, true, nil
	case !r.unowned(rec):
		r.logger.InfoContext(ctx, "forgetting the lease of what something owns now, which is a root no more", "kind", root.Kind, "uuid", root.UUID, "owners", rec.Metadata.Owning(r.parentOf(rec.Kind)))

		return graph.Ended{}, true, nil
	}

	requirement, err := rooted.Root(rec.Raw)
	if err != nil {
		return r.failed(fmt.Errorf("what the root needs cannot be read off its record: %w", err))
	}

	plan, err := r.planner.Plan(requirement)
	if err != nil {
		return r.failed(fmt.Errorf("its graph cannot be planned: %w", err))
	}

	// a record kept before generations is at its first.
	generation := max(rec.Metadata.Generation, 1)
	deleting := !rec.Metadata.DeletionRequestedAt.IsZero()

	outcome, err := r.held(ctx, root, func(ctx context.Context) (execute.Outcome, error) {
		if deleting {
			return r.executor.Remove(ctx, root, generation, plan)
		}

		return r.executor.Apply(ctx, root, generation, plan)
	})

	switch {
	case err != nil:
		return r.failed(err)
	case outcome.State == execute.Done && deleting:
		r.logger.InfoContext(ctx, "removed a root asked to be deleted, and what it owned", "kind", root.Kind, "uuid", root.UUID)

		return graph.Ended{}, true, nil
	case outcome.State == execute.Done:
		return graph.Ended{State: graph.StateDone}, false, nil
	case outcome.State == execute.Waiting:
		r.logger.DebugContext(ctx, "a root's plan waits on what it needs", "kind", root.Kind, "uuid", root.UUID, "reason", waitedOn(outcome))

		return graph.Ended{State: graph.StateWaiting, Reason: waitedOn(outcome)}, false, nil
	case outcome.State == execute.Failed:
		return r.failed(outcome.Err)
	}

	// cut short, by a lease lost or a control plane stopping, it is not
	// done, and is taken up again once the root is not backed off.
	return r.failed(fmt.Errorf("its plan was cut short: %s", interruptedBy(outcome)))
}

// held runs carry under root's lease, renewed every third of its time until
// carry returns: one lost meanwhile, which another control plane took once it
// lapsed, cancels the run, which stops.
func (r *Reconciler) held(ctx context.Context, root graph.Ref, carry func(ctx context.Context) (execute.Outcome, error)) (execute.Outcome, error) {
	ctx, cancel := context.WithCancelCause(ctx)
	defer cancel(nil)

	carried := make(chan struct{})
	renewing := make(chan struct{})

	go func() {
		defer close(renewing)

		ticker := time.NewTicker(r.config.Lease / 3)
		defer ticker.Stop()

		for {
			select {
			case <-carried:
				return
			case <-ctx.Done():
				return
			case <-ticker.C:
			}

			_, err := r.leases.Renew(ctx, root.UUID, r.holder, r.config.Lease)
			switch {
			case errors.Is(err, graph.ErrLost):
				r.logger.WarnContext(ctx, "lost a root's lease while its plan ran, which stops", "kind", root.Kind, "uuid", root.UUID)
				cancel(err)

				return
			case err != nil:
				r.logger.WarnContext(ctx, "could not renew a root's lease while its plan runs", "error", err, "kind", root.Kind, "uuid", root.UUID)
			}
		}
	}()

	outcome, err := carry(ctx)

	close(carried)
	<-renewing

	return outcome, err
}

// backedOff is root's lease not taken while root is backed off, err: a root
// that is gone meanwhile has its lease and how its plan went forgotten all
// the same, since nothing would take its lease again to forget it.
func (r *Reconciler) backedOff(ctx context.Context, root graph.Ref, lease graph.Lease, err error) error {
	_, there, readErr := r.read(ctx, root)
	if readErr != nil || there {
		r.logger.DebugContext(ctx, "a root is backed off after its plan failed", "kind", root.Kind, "uuid", root.UUID, "failures", lease.Failures, "next_at", lease.NextAt)

		return err
	}

	r.logger.InfoContext(ctx, "forgetting a root that is gone while it was backed off, its lease and how its plan went", "kind", root.Kind, "uuid", root.UUID)

	return r.forget(ctx, root)
}

// forget forgets how root's plan went, and then its lease, once root is
// gone, whatever becomes of ctx. What cannot be forgotten is logged, and is
// what it fails with: how its plan went kept, its lease is let go of as
// failed, and forgotten next time.
func (r *Reconciler) forget(ctx context.Context, root graph.Ref) error {
	ctx = context.WithoutCancel(ctx)

	if err := r.executions.DeleteByRoot(ctx, root.UUID); err != nil {
		r.logger.ErrorContext(ctx, "could not forget how a root's plan went", "error", err, "kind", root.Kind, "uuid", root.UUID)

		ended, _, _ := r.failed(fmt.Errorf("how its plan went cannot be forgotten: %w", err))
		_, releaseErr := r.leases.Release(ctx, root.UUID, r.holder, ended)

		return errors.Join(err, ignoreLost(releaseErr))
	}

	if err := r.leases.Forget(ctx, root.UUID); err != nil {
		r.logger.ErrorContext(ctx, "could not forget a root's lease", "error", err, "kind", root.Kind, "uuid", root.UUID)

		return err
	}

	return nil
}

// failed is a run that failed with err, as its lease keeps it, backing its
// root off as the reconciler is configured to.
func (r *Reconciler) failed(err error) (graph.Ended, bool, error) {
	return graph.Ended{State: graph.StateFailed, Reason: err.Error(), Backoff: r.config.Backoff}, false, err
}

// read is the record ref names, and whether there is one.
func (r *Reconciler) read(ctx context.Context, ref graph.Ref) (resource.Record, bool, error) {
	rec, err := r.resources.GetOne(ctx, ref.Kind, ref.UUID)
	switch {
	case errors.Is(err, domain.ErrNotExists):
		return resource.Record{}, false, nil
	case err != nil:
		return resource.Record{}, false, err
	}

	return rec, true, nil
}

// unowned reports whether nothing owns rec, as its metadata says: whether it
// is a root, being of a kind of root.
func (r *Reconciler) unowned(rec resource.Record) bool {
	return len(rec.Metadata.Owning(r.parentOf(rec.Kind))) == 0
}

// parentOf is the kind a resource of the named kind lives in, or nothing.
func (r *Reconciler) parentOf(kindName string) string {
	binding, registered := r.registry.Lookup(kindName)
	if !registered {
		return ""
	}

	return binding.Descriptor().Parent
}

// waitedOn is what the first node of outcome that waits waits on, by its
// node.
func waitedOn(outcome execute.Outcome) string {
	for _, e := range outcome.Nodes {
		if e.State == graph.StateWaiting {
			return e.Node.String() + ": " + e.Reason
		}
	}

	return ""
}

// interruptedBy is what cut outcome short, as the first node of it it cut
// short says, by its node.
func interruptedBy(outcome execute.Outcome) string {
	for _, e := range outcome.Nodes {
		if e.State == graph.StatePending && len(e.Reason) > 0 {
			return e.Node.String() + ": " + e.Reason
		}
	}

	return "it stopped between its levels"
}

// ignoreLost is err, but nothing for a lease that was lost, which another
// control plane holds now.
func ignoreLost(err error) error {
	if errors.Is(err, graph.ErrLost) {
		return nil
	}

	return err
}
