// Package reconcile keeps every root's graph made for as long as the root
// lives: a stack, or a container no stack owns (graph.RootKind), whose graph
// is planned from its record and carried out again whenever what it needs
// changed, and every so often besides.
//
// A root is reconciled (Reconcile) under its lease (graph.Leases), which one
// control plane holds at a time, renewed while the root's plan runs, or while
// it is removed with none, so that two never carry one root out at once: a root
// another holds is skipped, its lease poked, so that its holder carries it out
// again once its run ends, and nothing it was reconciled for is lost; and so is
// one backed off after its plan failed, until its backoff is over, but for one
// a person asked something of since its failed run read it, as its lease keeps
// what that run saw (a run cut short, or one that could not read the root,
// saw nothing), which is carried out at once. A root asked to be deleted
// is removed, from the top down, through what it owns
// (execute.Executor.Remove), however often it is reconciled, and whatever its
// failures backed it off for, and is never made again: one whose plan cannot be
// made, its record saying nothing a plan is made of, through what its records
// say it owns; any other is applied at its record's generation
// (execute.Executor.Apply), which makes what is missing of it, from the bottom
// up, and nothing else. A root a restore of what it lives in reset, of a kind
// that says what the restored parent holds of it (graph.ResetKind), a stack in
// a Docker VM whose disk was restored, is carried out only once the restored
// parent is found to hold it, forgotten with what it owns once it is found not
// to, nothing asked of a node, and waits until then, nothing made for it. How
// its plan ended is kept on its lease as it is let go of: one that failed
// backs the root off, longer with every failure in a row, one done clears
// that, and one that waits on what it needs leaves it. While the root itself
// is not made, why its plan failed or waits is said on its record too, a
// refusal as its failure, by its code, for whoever waits for it to be made. A
// root that is gone has its lease and how its plan went forgotten, one that
// went while its plan ran among them, which nothing makes again.
//
// What is reconciled is queued (Enqueue), each root once however often it is
// queued before it is reconciled, and reconciled by as many workers as the
// reconciler is given (Work), a root by one of them at a time. A record that
// changed is told to the reconciler (Changed, its graph.Notifier), which
// queues the roots it reaches through what owns it and what uses it, and
// what owns and uses those; and every so often every root is queued (Resync),
// which catches up with whatever no change was told of, and removes what
// records owned that are all gone, the end of a delete cut short after its
// root went. The same walk says which live roots need a record, and whether
// each makes it again (Dependents, its graph.Dependents), queueing nothing:
// what a record's delete answers with, and takes with it when it is asked to.
// What carrying a root's graph out, or removing it, would do now is read and
// not done (PreviewCreate, PreviewRemove): a dry run's answer.
//
// It runs the kinds it is given as roots, and no others: given none, nothing
// is queued, read or reconciled.
package reconcile

import (
	"context"
	"encoding/json"
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

// errCutShort is a run that stopped before its plan was done, its lease lost
// or its control plane stopping: no failure of its plan, and one that keeps
// nothing of what it saw of its root (asked).
var errCutShort = errors.New("its plan was cut short")

// errRecords is a run that could not read or write what the control plane
// keeps of its root and its plan, the root's record or how a node of its
// plan went, the database gone say: no failure of what its plan carries
// out, which a refusal is, or what a kind fails with.
var errRecords = errors.New("what the control plane keeps of it could not be read or written")

// tries is how many times a root's record is read and written again, when
// something else wrote it in the meantime, to say why its plan fails or
// waits.
const tries = 3

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

var _ graph.Graphs = &Reconciler{}

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
// ErrHeld, its lease poked, so that its holder carries it out again once it
// lets go of it; and so is one backed off, ErrBackedOff, unless it is gone
// meanwhile, when its lease is forgotten, since nothing would take it again to
// forget it, or it was asked to go, or asked something its graph is yet to
// carry out since the run whose failure backs it off read it, a stack's start
// say, which what its plan failed with before holds back nothing of: its lease
// is taken now, its backoff cleared in the same write, so that of two control
// planes that come to it at once, one takes it. Then its record is read. One
// that is gone, or that something owns now and is a root no more, has its
// lease and how its plan went forgotten, and that is all. One a restore of
// what it lives in reset, not asked to go, is carried out only once its kind
// finds the restored parent holds it, no longer reset then; it is forgotten,
// with what it owns, once its kind finds it does not, and its lease and how
// its plan went with it; and it waits until either, nothing of its plan
// carried out (graph.ResetKind). Any other's graph is planned, from what its
// kind says it needs (graph.RootKind.Root), and carried out while its lease
// is renewed every third of its time: removed when it was asked to be
// deleted, a delete cut short taken up where it stopped, and applied at its
// record's generation otherwise, a record kept before generations at its
// first; one asked to be deleted whose graph cannot be planned is removed
// through what its records say it owns, under its lease all the same; and a
// lease lost meanwhile, which another control plane took once it lapsed,
// stops the run.
//
// How it ended is kept on its lease as it is let go of: done, waiting, or
// failed, which a plan that could not be made is, and a run that was
// interrupted too, saying why, and, but for a run cut short, what it saw of
// the root as it read it; and a root removed has its lease forgotten. A root
// asked something after its run read it, which that run says nothing of
// (explain), or whose lease another control plane poked meanwhile, is queued
// to run again once its lease is let go of. It is nothing when its plan is
// done, waits, or it is gone, and what failed otherwise, which is logged with
// its kind and its uuid, and how long it is backed off for. A root of a kind
// that is not a root here is kind.ErrUnknownKind, and nothing is done.
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
	if errors.Is(err, graph.ErrBackedOff) {
		lease, err = r.goingNow(ctx, root, lease, err)
	}

	switch {
	case errors.Is(err, graph.ErrBackedOff):
		return r.backedOff(ctx, root, lease, err)
	case errors.Is(err, graph.ErrHeld):
		r.logger.DebugContext(ctx, "a root is carried out by another control plane, poked to carry it out again once its run ends", "kind", root.Kind, "uuid", root.UUID, "holder", lease.Holder)

		return err
	case err != nil:
		r.logger.ErrorContext(ctx, "could not take a root's lease", "error", err, "kind", root.Kind, "uuid", root.UUID)

		return err
	}

	read, ended, gone, err := r.carry(ctx, root, rooted)
	if gone {
		return r.forget(ctx, root)
	}

	again := r.explain(context.WithoutCancel(ctx), root, read, lease.Reason, ended, err)

	// what a failed run saw of its root is kept on its lease, which tells
	// what is asked of the root since from what that run left as it was. A
	// run cut short keeps nothing of it: what a person asked of the root,
	// which it was carrying out, is still to be carried out, and is taken at
	// once by whichever control plane comes to it next.
	if ended.State == graph.StateFailed && !errors.Is(err, errCutShort) {
		ended.Seen = seenOf(read)
	}

	released, releaseErr := r.leases.Release(context.WithoutCancel(ctx), root.UUID, r.holder, ended)

	// what it was asked after its plan's run read it, here or on another
	// control plane, which poked its lease, is carried out by the next.
	if again || released.Poked {
		r.Enqueue(root)
	}

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

// carry carries out the plan of root, whose lease is taken, and is the
// root's record as the run read it before it carried its plan out, none when
// it read none, and how it ended, as its lease keeps it, or whether its lease
// is to be forgotten instead, and what failed.
func (r *Reconciler) carry(ctx context.Context, root graph.Ref, rooted graph.RootKind) (resource.Record, graph.Ended, bool, error) {
	rec, there, err := r.read(ctx, root)
	switch {
	case err != nil:
		ended, gone, err := r.failed(fmt.Errorf("%w: %w", errRecords, err))

		return resource.Record{}, ended, gone, err
	case !there:
		r.logger.InfoContext(ctx, "forgetting a root that is gone, its lease and how its plan went", "kind", root.Kind, "uuid", root.UUID)

		return resource.Record{}, graph.Ended{}, true, nil
	case !r.unowned(rec):
		r.logger.InfoContext(ctx, "forgetting the lease of what something owns now, which is a root no more", "kind", root.Kind, "uuid", root.UUID, "owners", rec.Metadata.Owning(r.parentOf(rec.Kind)))

		return rec, graph.Ended{}, true, nil
	}

	ended, gone, err := r.carried(ctx, root, rooted, rec)

	return rec, ended, gone, err
}

// carried carries out the plan of root, whose record, rec, a root's that
// nothing owns, the run read, as carry says. One asked to go whose plan
// cannot be made is removed all the same, through what its records say it
// owns (removeUnplanned), under its lease as a plan is carried out (held).
func (r *Reconciler) carried(ctx context.Context, root graph.Ref, rooted graph.RootKind, rec resource.Record) (graph.Ended, bool, error) {
	deleting := !rec.Metadata.DeletionRequestedAt.IsZero()

	// one a restore of what it lives in reset is carried out once its kind
	// finds the restored parent holds it, forgotten once it finds it does
	// not, and waits until then (restored).
	if resetting, resets := rooted.(graph.ResetKind); resets && rec.Reset && !deleting {
		held, ended, gone, err := r.restored(ctx, root, resetting, rec)
		if held == nil {
			return ended, gone, err
		}

		rec = *held
	}

	unplanned := func(cause error) (ended graph.Ended, gone bool, err error) {
		if !deleting {
			return r.failed(cause)
		}

		r.held(ctx, root, func(ctx context.Context) {
			ended, gone, err = r.removeUnplanned(ctx, root, rec, cause)
		})

		return ended, gone, err
	}

	requirement, err := rooted.Root(rec.Raw)
	if err != nil {
		return unplanned(fmt.Errorf("what the root needs cannot be read off its record: %w", err))
	}

	plan, err := r.planner.Plan(requirement)
	if err != nil {
		return unplanned(fmt.Errorf("its graph cannot be planned: %w", err))
	}

	// a record kept before generations is at its first.
	generation := max(rec.Metadata.Generation, 1)

	var outcome execute.Outcome

	r.held(ctx, root, func(ctx context.Context) {
		if deleting {
			outcome, err = r.executor.Remove(ctx, root, generation, plan)
		} else {
			outcome, err = r.executor.Apply(ctx, root, generation, plan, execute.While(r.wanted(root, generation)))
		}
	})

	// gone while its plan ran, it is not made again, whatever is left of its
	// plan (wanted): it is forgotten, as a root found gone is. Changed while
	// it ran, at another generation, its graph is planned again by the run
	// after, which it is queued for.
	if !deleting && err == nil && outcome.State != execute.Done {
		read, there, readErr := r.read(ctx, root)

		switch {
		case readErr != nil:
		case !there:
			r.logger.InfoContext(ctx, "forgetting a root that went while its plan ran, its lease and how its plan went", "kind", root.Kind, "uuid", root.UUID)

			return graph.Ended{}, true, nil
		case max(read.Metadata.Generation, 1) != generation:
			r.Enqueue(root)
		}
	}

	// what the executor fails with beside how far the plan went is how its
	// nodes went not being kept, and no failure of any of them: a node that
	// failed, refused say, is what the run failed with all the same, and what
	// could not be kept is logged beside it.
	switch {
	case outcome.State == execute.Failed:
		if err != nil {
			r.logger.WarnContext(ctx, "could not keep how the nodes of a root's plan went, which failed", "error", err, "kind", root.Kind, "uuid", root.UUID)
		}

		return r.failed(outcome.Err)
	case err != nil:
		return r.failed(fmt.Errorf("%w: %w", errRecords, err))
	case outcome.State == execute.Done && deleting:
		r.logger.InfoContext(ctx, "removed a root asked to be deleted, and what it owned", "kind", root.Kind, "uuid", root.UUID)

		return graph.Ended{}, true, nil
	case outcome.State == execute.Done:
		return graph.Ended{State: graph.StateDone}, false, nil
	case outcome.State == execute.Waiting:
		r.logger.DebugContext(ctx, "a root's plan waits on what it needs", "kind", root.Kind, "uuid", root.UUID, "reason", waitedOn(outcome))

		return graph.Ended{State: graph.StateWaiting, Reason: waitedOn(outcome)}, false, nil
	}

	// cut short, by a lease lost or a control plane stopping, it is not
	// done, and is taken up again once the root is not backed off.
	return r.failed(fmt.Errorf("%w: %s", errCutShort, interruptedBy(outcome)))
}

// wanted is what a run applying the plan of root, made at generation, asks
// before it makes what is not there: whether root is still to be made as that
// plan says, its record there, at that generation, and not asked to go, nor
// reset by a restore of what it lives in that its kind waits to find it in
// (graph.ResetKind). One whose record a run read before it was asked to go,
// or went, has nothing made for it after, so that what a cascade took away
// with it, its VM first among it, is not made again by a run that started
// before; one changed since the run read it has nothing made of a plan that
// says what it was, as its VM's delete changes a root it keeps, to make the
// VM again as it was, before the VM's record goes, where a run that planned
// it before would make another VM in its place, named by nothing but its
// uuid; and one a restore reset meanwhile has nothing made for it beside what
// the restored parent may hold of it. What is made is waited on instead,
// saying why, and the run ends.
func (r *Reconciler) wanted(root graph.Ref, generation int64) func(ctx context.Context) error {
	return func(ctx context.Context) error {
		rec, there, err := r.read(ctx, root)
		switch {
		case err != nil:
			return err
		case !there:
			return &graph.Waiting{Reason: "its root is gone"}
		case going(rec):
			return &graph.Waiting{Reason: "its root was asked to go"}
		case max(rec.Metadata.Generation, 1) != generation:
			return &graph.Waiting{Reason: "its root was changed since its plan was made, which is made again"}
		case rec.Reset && r.resets(root.Kind):
			return &graph.Waiting{Reason: fmt.Sprintf("its root's %s was restored, and what the restored %s holds of its root is yet to be known", r.parentOf(root.Kind), r.parentOf(root.Kind))}
		}

		return nil
	}
}

// explain writes onto the record of root why its plan failed or waits, as
// ended says, while the root itself is not made, in its kind's first state or
// missing: why goes in its status's reason, and a refusal (graph.Refused), as
// its own admission would have refused it, in its failure, by its code and
// with that reason, which whoever waits for the root to be made reads as
// what it was refused for. What failed for any other reason, the database or
// a timeout, is a reason only. What a plan said of it is let go of once a
// later plan says otherwise: its failure, which a plan said when its message
// is what last failed its plan, as its lease kept it (last), is cleared once
// a plan is done, or fails or waits for another reason. A root that is made
// is its node's to say of, and so is one asked to go; and nothing is written
// that is what is written already. What reading or writing fails with is
// logged, and is said again by the next run.
//
// A root whose state the control plane keeps (kind.OnControlPlane), a stack,
// has no node to say what it is doing but its own in its plan: whatever it
// is doing, a plan that failed fails it, and one that waits on what it needs
// has it wait, as its machine takes either from where it is, and says why. A
// run cut short, its lease lost or its control plane stopping, is no failure
// of it, and says nothing of it; nor is a run that could not read or write
// what the control plane keeps of it, its record or how its plan's nodes
// went, which fails nothing its plan carries out; nor does a run say
// anything of a state that began after it read the root, as read (none when
// it read none), which something else wrote meanwhile, a person's start say:
// that is left as it is, and explain reports that the root is to run again,
// to carry it out.
func (r *Reconciler) explain(ctx context.Context, root graph.Ref, read resource.Record, last string, ended graph.Ended, failure error) bool {
	for try := 1; try <= tries; try++ {
		rec, there, err := r.read(ctx, root)
		if err != nil || !there || going(rec) {
			return false
		}

		explained, changed, again, err := r.explained(rec, read, last, ended, failure)
		if err != nil {
			r.logger.WarnContext(ctx, "could not say on a root why its plan fails or waits", "error", err, "kind", root.Kind, "uuid", root.UUID)

			return false
		}

		if again {
			r.logger.InfoContext(ctx, "a root was asked something after its plan's run read it, which the next run carries out", "kind", root.Kind, "uuid", root.UUID)

			return true
		}

		if !changed {
			return false
		}

		explained.Metadata.UpdatedAt = time.Now()

		_, err = r.resources.Update(ctx, explained)
		switch {
		case err == nil, errors.Is(err, domain.ErrNotExists):
			return false
		case !errors.Is(err, resource.ErrConflict):
			r.logger.WarnContext(ctx, "could not say on a root why its plan fails or waits", "error", err, "kind", root.Kind, "uuid", root.UUID)

			return false
		}
	}

	return false
}

// explained is rec, the record of a root, saying why its plan failed or
// waits, as explain says, and whether that changed it, or whether it is left
// as it is to run again, its state having begun after the run read it, as
// read.
func (r *Reconciler) explained(rec resource.Record, read resource.Record, last string, ended graph.Ended, failure error) (resource.Record, bool, bool, error) {
	common, err := rec.Common()
	if err != nil {
		return resource.Record{}, false, false, err
	}

	said, err := failureOf(rec.Status)
	if err != nil {
		return resource.Record{}, false, false, err
	}

	// what a plan said is what last failed its plan.
	ours := len(said.Code) > 0 && len(last) > 0 && said.Message == last

	var refused *graph.Refused

	wanted := said

	switch {
	case ended.State == graph.StateFailed && errors.As(failure, &refused):
		wanted = planFailure{Code: refused.Code, Message: ended.Reason}
	case ours:
		wanted = planFailure{}
	}

	changed := false

	unmade := common.State == kind.Missing
	if binding, registered := r.registry.Lookup(rec.Kind); registered {
		d := binding.Descriptor()
		unmade = unmade || common.State == d.Machine.Initial

		// nothing but its plan says what a root the control plane keeps the
		// state of is doing, and a run cut short says nothing of it, nor does
		// one that could not read or write what is kept of it, nor one of a
		// state that began after it read the root.
		if d.StateBy == kind.OnControlPlane && ended.State != graph.StateDone && !errors.Is(failure, errCutShort) && !errors.Is(failure, errRecords) {
			if began(read, common) {
				return rec, false, true, nil
			}

			unmade = true

			if moved := settled(d, &common, ended); moved {
				changed = true

				if err := rec.SetCommon(common); err != nil {
					return resource.Record{}, false, false, err
				}
			}
		}
	}

	// a root made says what it is doing itself, or its node does: what a plan
	// said of it is let go of, and nothing more is said.
	if !unmade || ended.State == graph.StateDone {
		if !ours {
			return rec, false, false, nil
		}

		wanted = planFailure{}
	} else if common.Reason != ended.Reason {
		common.Reason = ended.Reason
		changed = true

		if err := rec.SetCommon(common); err != nil {
			return resource.Record{}, false, false, err
		}
	}

	if wanted != said {
		status, err := resource.Merge(rec.Status, failureStatus(wanted))
		if err != nil {
			return resource.Record{}, false, false, err
		}

		rec.Status = status
		changed = true
	}

	return rec, changed, false, nil
}

// began reports whether a root's state, now, began after a run read its
// record, as read: whether it is another state since, or the same one begun
// again, which something else wrote meanwhile. A run that read no record is
// one that could not read what the control plane keeps, which says nothing
// of its root (explain), and is never asked.
func began(read resource.Record, now kind.Status) bool {
	then, err := read.Common()

	return err != nil || then.State != now.State || !then.Since.Equal(now.Since)
}

// settled moves common, the status of a root whose state the control plane
// keeps, as its machine, d's, takes how its plan ended: failed once it
// failed, and waiting once it waits. It reports whether it moved.
func settled(d kind.Descriptor, common *kind.Status, ended graph.Ended) bool {
	observed := kind.Waiting
	if ended.State == graph.StateFailed {
		observed = kind.Failed
	}

	to, moved := d.Machine.Next(common.State, kind.OnObserved(observed))
	if !moved || to == common.State {
		return false
	}

	common.State = to
	common.Since = time.Now()

	return true
}

// planFailure is what a root's status says its last command, or its plan,
// failed with: a code, and what was said of it, in the words every side
// knows. An empty one is none.
type planFailure struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

// failureOf is what status says its resource failed with, or none.
func failureOf(status json.RawMessage) (planFailure, error) {
	var said struct {
		Failure *planFailure `json:"failure"`
	}

	if len(status) == 0 {
		return planFailure{}, nil
	}

	if err := json.Unmarshal(status, &said); err != nil {
		return planFailure{}, err
	}

	if said.Failure == nil {
		return planFailure{}, nil
	}

	return *said.Failure, nil
}

// failureStatus is a status that says nothing but that its resource failed
// with failure, which an empty one says it did not.
func failureStatus(failure planFailure) json.RawMessage {
	status, err := json.Marshal(struct {
		Failure planFailure `json:"failure"`
	}{Failure: failure})
	if err != nil {
		panic(err)
	}

	return status
}

// held runs carry under root's lease, renewed every third of its time until
// carry returns: one lost meanwhile, which another control plane took once it
// lapsed, cancels the run, which stops. What carry comes to is its own to
// keep.
func (r *Reconciler) held(ctx context.Context, root graph.Ref, carry func(ctx context.Context)) {
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

	carry(ctx)

	close(carried)
	<-renewing
}

// goingNow is root's lease, which was not taken while root is backed off,
// err, taken now after all, its backoff cleared in the same write
// (graph.Leases.TakeNow), when root was asked to go, or asked something its
// graph is yet to carry out since the run whose failure backs it off read it
// (asked), a stack's restart say: what failed of its plan before holds back
// nothing of what a person asked of it since. Of the control planes that
// come to it at once, one takes it, and the others find it held. Any other
// root's is left as it was.
func (r *Reconciler) goingNow(ctx context.Context, root graph.Ref, lease graph.Lease, err error) (graph.Lease, error) {
	rec, there, readErr := r.read(ctx, root)
	if readErr != nil || !there {
		return lease, err
	}

	going := !rec.Metadata.DeletionRequestedAt.IsZero()
	if !going && !r.asked(rec, lease.Seen) {
		return lease, err
	}

	if going {
		r.logger.InfoContext(ctx, "a root asked to go is removed whatever its plan's failures backed it off for", "kind", root.Kind, "uuid", root.UUID, "failures", lease.Failures)
	} else {
		r.logger.InfoContext(ctx, "what a root was asked is carried out now, whatever its plan's failures backed it off for", "kind", root.Kind, "uuid", root.UUID, "failures", lease.Failures)
	}

	return r.leases.TakeNow(ctx, root.UUID, r.holder, r.config.Lease)
}

// asked reports whether rec, the record of a root, was asked something its
// graph is yet to carry out since the run whose failure backs it off read it,
// as it saw it (seen): its state is the control plane's, one in flight that
// an action of its kind moves it into, a stack starting, stopping or
// restarting, and not the one that run saw, begun when it saw it. A run that
// fails to carry it out fails it, as explain says, and leaves it so when it
// was asked again after that run read it, or when the run, having read it,
// could not keep how its plan went, what the control plane keeps out of
// reach: the one is carried out at once, and the other is left until its
// backoff is over, so that a root is carried out at once only for what a
// person asked of it since its plan last failed. A run that saw nothing of it
// keeps nothing (seen is none), and it is carried out at once while it is in
// flight so: one cut short, its lease lost or its control plane stopping,
// which leaves what a person asked of it as it was asked, so that whichever
// control plane comes to it next carries that out, whatever its failures
// before; and one that could not read its record at all, after which each
// trigger that can read it carries it out, no more often than it is
// triggered. A stack's restart, carried out by its run, moves it into
// starting too, a state that run did not see: should the run fail after it,
// the stack is carried out again at once, and the run after it, which sees it
// starting, fails it.
func (r *Reconciler) asked(rec resource.Record, seen graph.Seen) bool {
	binding, registered := r.registry.Lookup(rec.Kind)
	if !registered {
		return false
	}

	d := binding.Descriptor()

	common, err := rec.Common()
	if err != nil || d.StateBy != kind.OnControlPlane || !d.Machine.IsInFlight(common.State) {
		return false
	}

	if seen.Is(graph.Seen{State: common.State, Since: common.Since}) {
		return false
	}

	return slices.ContainsFunc(d.Machine.Transitions, func(t kind.Transition) bool {
		return t.To == common.State && len(t.On.Action) > 0
	})
}

// seenOf is what read, the record of a root as a run read it, said the root
// was doing: none when the run read none, or what it says cannot be read.
func seenOf(read resource.Record) graph.Seen {
	if len(read.Metadata.UUID) == 0 {
		return graph.Seen{}
	}

	common, err := read.Common()
	if err != nil {
		return graph.Seen{}
	}

	return graph.Seen{State: common.State, Since: common.Since}
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
