package graph

import (
	"context"
	"slices"
	"sync/atomic"

	"github.com/khanzadimahdi/testproject/domain/workload/kind"
)

// RootKind is a kind of a graph whose resources are roots of graphs of their
// own: each that nothing owns, as its record says (kind.Metadata.Owning),
// whose graph is planned from it, and carried out for as long as it lives. A
// stack is one, and so is a container no stack owns.
type RootKind interface {
	Kind

	// Root is the requirement a root of the kind is, as its record keeps
	// it: what its graph is planned from. It reads nothing but r, and a
	// record that is not of the kind, or that does not say what the root
	// needs, is an error.
	Root(r kind.Raw) (Requirement, error)
}

// Roots are the kinds whose resources are roots of graphs of their own, by
// their names: each resource of one that nothing owns is a root, whose graph
// makes it and what it needs, in order, for as long as it lives, and removes
// it, and what it owns, once it is asked to go. What the control plane does
// beside the graphs, its admission, its delete, its cascade and its
// reconcile loop, leaves both to a root's graph.
type Roots []string

// Root reports whether a record of the kind d describes, whose metadata m
// is, is a root: of a kind among them, and owned by nothing but the resource
// it lives in (kind.Metadata.Owning).
func (r Roots) Root(d kind.Descriptor, m kind.Metadata) bool {
	return slices.Contains(r, d.Name) && len(m.Owning(d.Parent)) == 0
}

// Notifier is told that a record a graph's node is changed: it went missing,
// was deleted, was made or failed. Whatever it changed for, the roots whose
// graphs it is in are carried out again, so that what they need is made
// again, in order, as their plans say.
type Notifier interface {
	// Changed is told the record ref names changed. What needs it, through
	// what owns it and what uses it, and what needs that, are looked for up
	// to the roots it reaches, and each root is carried out again: a root
	// changed is carried out itself. What reading the records fails with is
	// an error, and the roots found by then are carried out all the same.
	Changed(ctx context.Context, ref Ref) error
}

// Enqueuer is told a root is to be carried out now, rather than at the next
// resync: one just admitted, or just asked to go.
type Enqueuer interface {
	// Enqueue queues root to be carried out, once however often it is
	// queued before it is. What is not a root is not queued.
	Enqueue(root Ref)
}

// Dependent is a root whose graph needs a record: one of the nodes of its
// plan is the record.
type Dependent struct {
	// Root is the root, and Name what it is called.
	Root Ref
	Name string

	// Recreates says its graph makes the record again whenever it is gone,
	// a node of its own. One that does not only names it, as what was there
	// before it, made by somebody else (Requirement.External), and waits on
	// it while it is missing.
	Recreates bool
}

// Dependents are asked which roots need a record, as a node of their plans.
type Dependents interface {
	// Dependents are the roots whose graphs need the record ref names, each
	// once, in the order of their kinds and their uuids: those it reaches
	// up through what owns it and what uses it, as Notifier.Changed does,
	// whose plans have it among their nodes, and that were not asked to go.
	// A root is not its own dependent. What reading or planning fails with is
	// an error, and nothing is queued, read for its own sake or written.
	Dependents(ctx context.Context, ref Ref) ([]Dependent, error)
}

// Graphs are what carries the roots out as it is told of them: queued when
// they are admitted or asked to go, and reached from what changed under them;
// and what says which of them need a record.
type Graphs interface {
	Notifier
	Enqueuer
	Dependents
}

// Relay is what the control plane tells of roots, and of what changes under
// them, and asks which roots need a record, before what carries the roots out
// is built over it: it tells and asks the graphs it is given (To), from then
// on, and nobody until then.
type Relay struct {
	to atomic.Pointer[Graphs]
}

var _ Graphs = &Relay{}

// To has the relay tell graphs from now on.
func (r *Relay) To(graphs Graphs) {
	r.to.Store(&graphs)
}

// Changed tells the graphs the relay tells that the record ref names
// changed, and is nothing before it tells any.
func (r *Relay) Changed(ctx context.Context, ref Ref) error {
	graphs := r.to.Load()
	if graphs == nil {
		return nil
	}

	return (*graphs).Changed(ctx, ref)
}

// Enqueue queues root to the graphs the relay tells, and to nothing before
// it tells any.
func (r *Relay) Enqueue(root Ref) {
	if graphs := r.to.Load(); graphs != nil {
		(*graphs).Enqueue(root)
	}
}

// Dependents are the roots that need the record ref names, as the graphs the
// relay tells say, and none before it tells any.
func (r *Relay) Dependents(ctx context.Context, ref Ref) ([]Dependent, error) {
	graphs := r.to.Load()
	if graphs == nil {
		return nil, nil
	}

	return (*graphs).Dependents(ctx, ref)
}
