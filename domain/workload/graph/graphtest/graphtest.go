// Package graphtest is a graph.Kind a test says everything of (Kind), which
// may stand for every kind of a graph at once, and what owns the records it
// holds (Ownership): what the tests of a plan, and of what carries one out,
// are written against.
package graphtest

import (
	"cmp"
	"context"
	"fmt"
	"maps"
	"slices"
	"sync"

	"github.com/khanzadimahdi/testproject/domain/workload/graph"
	"github.com/khanzadimahdi/testproject/domain/workload/kind"
)

// Op is what a kind, or what owns its records, is asked of a resource.
type Op string

const (
	// Requires is asking what it needs, Exists whether it is there, Create
	// to make it and Delete to remove it.
	Requires Op = "requires"
	Exists   Op = "exists"
	Create   Op = "create"
	Delete   Op = "delete"

	// Owners is asking what owns its record, Disown to take owners out of
	// what does, and Forget to remove a root's record.
	Owners Op = "owners"
	Disown Op = "disown"
	Forget Op = "forget"
)

// Call is a kind, or what owns its records, asked something of a node: Deps
// are the records of what it needs that a create was handed, and Owners the
// owners a disown was handed.
type Call struct {
	Op     Op
	Node   graph.NodeID
	Deps   map[graph.NodeID]graph.Ref
	Owners []graph.Ref
}

// Event is a call beginning, or, when Returned, returning.
type Event struct {
	Op       Op
	Node     graph.NodeID
	Returned bool
}

// Kind is a graph.Kind a test says everything of. A resource's spec is its
// node's id (Requirement), and its record is its kind and its name (Ref), so
// one Kind may be every kind of a graph at once (Kinds), and what they were
// all asked is in one order.
//
// What a node requires is what Require said, and nothing else. Whether there
// is a record of it, and whether it is made, is what the kind holds: Create
// holds it made, Delete lets it go, or leaves it, not made, when a test says
// so (Leave), Hold holds it as if it were made before, and Record holds a
// record of it that is not made; one that is not held is removed all the
// same. What owns each record is what a test says (Ownership), and goes with
// the record. What a test has a node do when it is asked something (On) is
// done first, and what that fails with is what it is answered. Everything
// the kind, and what owns its records, is asked is recorded (Calls), and when
// each call began and returned (Events), and it may be asked from several
// goroutines at once.
type Kind struct {
	lock     sync.Mutex
	requires map[graph.NodeID][]graph.Requirement

	// records are the nodes it holds a record of, and whether each is made,
	// and owners what each record names as its owners; left are the nodes
	// whose records a delete leaves.
	records map[graph.NodeID]bool
	owners  map[graph.NodeID][]graph.NodeID
	left    map[graph.NodeID]bool

	on     map[Op]map[graph.NodeID]func(ctx context.Context) error
	calls  []Call
	events []Event
}

var _ graph.Kind = &Kind{}

// NewKind is a kind that holds nothing, and whose every node requires
// nothing.
func NewKind() *Kind {
	return &Kind{
		requires: make(map[graph.NodeID][]graph.Requirement),
		records:  make(map[graph.NodeID]bool),
		owners:   make(map[graph.NodeID][]graph.NodeID),
		left:     make(map[graph.NodeID]bool),
		on:       make(map[Op]map[graph.NodeID]func(ctx context.Context) error),
	}
}

// Requirement is the node of a kind and a name, as a fake kind requires it:
// its spec is its id.
func Requirement(kindName string, name string) graph.Requirement {
	return graph.Requirement{Kind: kindName, Name: name, Spec: graph.ID(kindName, name)}
}

// Ref is the record a fake kind says the node id names is: its kind and its
// name.
func Ref(id graph.NodeID) graph.Ref {
	return graph.Ref{Kind: id.Kind(), UUID: id.Name()}
}

// Kinds are the kinds of the names, every one of them k.
func (k *Kind) Kinds(names ...string) graph.Kinds {
	kinds := make(graph.Kinds, len(names))
	for _, name := range names {
		kinds[name] = k
	}

	return kinds
}

// Require has the node id require what required says, in its order, in place
// of what it required before.
func (k *Kind) Require(id graph.NodeID, required ...graph.Requirement) {
	k.lock.Lock()
	defer k.lock.Unlock()

	k.requires[id] = slices.Clone(required)
}

// Hold holds the nodes ids name, as if each were made before.
func (k *Kind) Hold(ids ...graph.NodeID) {
	k.lock.Lock()
	defer k.lock.Unlock()

	for _, id := range ids {
		k.records[id] = true
	}
}

// Record holds a record of each node ids name that is not made: one being
// made, say, or that failed before it was, or that is going away.
func (k *Kind) Record(ids ...graph.NodeID) {
	k.lock.Lock()
	defer k.lock.Unlock()

	for _, id := range ids {
		k.records[id] = false
	}
}

// Leave has Delete leave the record of each node ids name, not made, rather
// than let it go: what a kind does whose record goes last of all, once what
// it owns is removed (graph.Ownership.Forget).
func (k *Kind) Leave(ids ...graph.NodeID) {
	k.lock.Lock()
	defer k.lock.Unlock()

	for _, id := range ids {
		k.left[id] = true
	}
}

// Held are the nodes the kind holds a record of, made or not, in the order
// of their ids.
func (k *Kind) Held() []graph.NodeID {
	k.lock.Lock()
	defer k.lock.Unlock()

	return slices.Sorted(maps.Keys(k.records))
}

// On has the node id do what do says whenever it is asked op, before
// anything else, and be answered with what it fails with when it fails: a
// test may block in it, or fail the node, or have it wait (graph.Waiting).
// Requires, which is handed no context, hands it an empty one. A nil do
// undoes it.
func (k *Kind) On(op Op, id graph.NodeID, do func(ctx context.Context) error) {
	k.lock.Lock()
	defer k.lock.Unlock()

	if do == nil {
		delete(k.on[op], id)

		return
	}

	if k.on[op] == nil {
		k.on[op] = make(map[graph.NodeID]func(ctx context.Context) error)
	}

	k.on[op][id] = do
}

// Calls are everything the kind, and what owns its records, was asked, in
// order.
func (k *Kind) Calls() []Call {
	k.lock.Lock()
	defer k.lock.Unlock()

	calls := make([]Call, 0, len(k.calls))
	for _, c := range k.calls {
		c.Deps = maps.Clone(c.Deps)
		c.Owners = slices.Clone(c.Owners)
		calls = append(calls, c)
	}

	return calls
}

// Events are when each call began, and when it returned, in the order they
// did: a call returns once what it does is done, so whatever began after it
// returned began after all of it.
func (k *Kind) Events() []Event {
	k.lock.Lock()
	defer k.lock.Unlock()

	return slices.Clone(k.events)
}

// Asked are the nodes op was asked of, in order, once each time.
func (k *Kind) Asked(op Op) []graph.NodeID {
	k.lock.Lock()
	defer k.lock.Unlock()

	var asked []graph.NodeID
	for _, c := range k.calls {
		if c.Op == op {
			asked = append(asked, c.Node)
		}
	}

	return asked
}

// Requires is what Require said the node spec names requires: nothing, when
// it said nothing.
func (k *Kind) Requires(spec any) ([]graph.Requirement, error) {
	id, err := nodeOf(spec)
	if err != nil {
		return nil, err
	}

	defer k.returned(Requires, id)

	if err := k.ask(context.Background(), Call{Op: Requires, Node: id}); err != nil {
		return nil, err
	}

	k.lock.Lock()
	defer k.lock.Unlock()

	return slices.Clone(k.requires[id]), nil
}

// Exists is the record of the node spec names, when the kind holds one, and
// whether it is made.
func (k *Kind) Exists(ctx context.Context, spec any) (graph.Ref, bool, error) {
	id, err := nodeOf(spec)
	if err != nil {
		return graph.Ref{}, false, err
	}

	defer k.returned(Exists, id)

	if err := k.ask(ctx, Call{Op: Exists, Node: id}); err != nil {
		return graph.Ref{}, false, err
	}

	k.lock.Lock()
	defer k.lock.Unlock()

	made, recorded := k.records[id]
	if !recorded {
		return graph.Ref{}, false, nil
	}

	return Ref(id), made, nil
}

// Create holds the node spec names, made, and is its record.
func (k *Kind) Create(ctx context.Context, spec any, deps map[graph.NodeID]graph.Ref) (graph.Ref, error) {
	id, err := nodeOf(spec)
	if err != nil {
		return graph.Ref{}, err
	}

	defer k.returned(Create, id)

	if err := k.ask(ctx, Call{Op: Create, Node: id, Deps: maps.Clone(deps)}); err != nil {
		return graph.Ref{}, err
	}

	k.lock.Lock()
	defer k.lock.Unlock()

	k.records[id] = true

	return Ref(id), nil
}

// Delete lets the node ref is the record of go, and what owns it with it, or
// leaves its record, not made, when a test says so (Leave).
func (k *Kind) Delete(ctx context.Context, ref graph.Ref) error {
	id := graph.ID(ref.Kind, ref.UUID)

	defer k.returned(Delete, id)

	if err := k.ask(ctx, Call{Op: Delete, Node: id}); err != nil {
		return err
	}

	k.lock.Lock()
	defer k.lock.Unlock()

	if _, recorded := k.records[id]; recorded && k.left[id] {
		k.records[id] = false

		return nil
	}

	k.forget(id)

	return nil
}

// Ownership is what owns the records k holds.
func (k *Kind) Ownership() *Ownership {
	return &Ownership{kind: k}
}

// ask records the call, and does what the test has its node do first,
// outside the lock, so that it may block.
func (k *Kind) ask(ctx context.Context, call Call) error {
	k.lock.Lock()
	k.calls = append(k.calls, call)
	k.events = append(k.events, Event{Op: call.Op, Node: call.Node})
	do := k.on[call.Op][call.Node]
	k.lock.Unlock()

	if do == nil {
		return nil
	}

	return do(ctx)
}

// returned records the node id's op returning, once everything it does is
// done.
func (k *Kind) returned(op Op, id graph.NodeID) {
	k.lock.Lock()
	defer k.lock.Unlock()

	k.events = append(k.events, Event{Op: op, Node: id, Returned: true})
}

// forget lets the record of the node id go, and what owns it with it. The
// lock is held.
func (k *Kind) forget(id graph.NodeID) {
	delete(k.records, id)
	delete(k.owners, id)
}

// Ownership is what owns the records a fake kind holds (graph.Ownership), as
// a test says (Own): an owner owns a record while the kind holds a record of
// it, and what owns a record goes with it. What it is asked is recorded
// among what the kind is asked, in one order, and what a test has a node do
// when it is asked something (Kind.On) is done first, as the kind's own
// calls are.
type Ownership struct {
	kind *Kind
}

var _ graph.Ownership = &Ownership{}

// Own has the record of the node id owned by the nodes owners name too,
// each once, whether or not the kind holds a record of any of them yet.
func (o *Ownership) Own(id graph.NodeID, owners ...graph.NodeID) {
	o.kind.lock.Lock()
	defer o.kind.lock.Unlock()

	for _, owner := range owners {
		if !slices.Contains(o.kind.owners[id], owner) {
			o.kind.owners[id] = append(o.kind.owners[id], owner)
		}
	}
}

// OwnersOf are what the record of the node id names as its owners, in the
// order of their ids, whether or not the kind holds their records.
func (o *Ownership) OwnersOf(id graph.NodeID) []graph.NodeID {
	o.kind.lock.Lock()
	defer o.kind.lock.Unlock()

	owners := slices.Clone(o.kind.owners[id])
	slices.Sort(owners)

	return owners
}

// Owners are the records that own ref's record that the kind holds, in the
// order of their kinds and their uuids: none when it holds no record of ref.
func (o *Ownership) Owners(ctx context.Context, ref graph.Ref) ([]graph.Ref, error) {
	id := graph.ID(ref.Kind, ref.UUID)

	defer o.kind.returned(Owners, id)

	if err := o.kind.ask(ctx, Call{Op: Owners, Node: id}); err != nil {
		return nil, err
	}

	o.kind.lock.Lock()
	defer o.kind.lock.Unlock()

	return o.owning(id), nil
}

// Disown takes owners out of what owns ref's record, and is what owns it
// still, as Owners says it: nothing when the kind holds no record of ref.
func (o *Ownership) Disown(ctx context.Context, ref graph.Ref, owners ...graph.Ref) ([]graph.Ref, error) {
	id := graph.ID(ref.Kind, ref.UUID)

	defer o.kind.returned(Disown, id)

	if err := o.kind.ask(ctx, Call{Op: Disown, Node: id, Owners: slices.Clone(owners)}); err != nil {
		return nil, err
	}

	o.kind.lock.Lock()
	defer o.kind.lock.Unlock()

	if _, recorded := o.kind.records[id]; !recorded {
		return nil, nil
	}

	o.kind.owners[id] = slices.DeleteFunc(o.kind.owners[id], func(owner graph.NodeID) bool {
		return slices.Contains(owners, Ref(owner))
	})

	return o.owning(id), nil
}

// Forget lets root's record go, and what owns it with it: one the kind does
// not hold is forgotten already.
func (o *Ownership) Forget(ctx context.Context, root graph.Ref) error {
	id := graph.ID(root.Kind, root.UUID)

	defer o.kind.returned(Forget, id)

	if err := o.kind.ask(ctx, Call{Op: Forget, Node: id}); err != nil {
		return err
	}

	o.kind.lock.Lock()
	defer o.kind.lock.Unlock()

	o.kind.forget(id)

	return nil
}

// owning are the records that own the record of the node id that the kind
// holds, in the order of their kinds and their uuids: none when it holds no
// record of the node. The lock is held.
func (o *Ownership) owning(id graph.NodeID) []graph.Ref {
	if _, recorded := o.kind.records[id]; !recorded {
		return nil
	}

	var owning []graph.Ref
	for _, owner := range o.kind.owners[id] {
		if _, there := o.kind.records[owner]; there {
			owning = append(owning, Ref(owner))
		}
	}

	slices.SortFunc(owning, func(a, b graph.Ref) int {
		return cmp.Or(cmp.Compare(a.Kind, b.Kind), cmp.Compare(a.UUID, b.UUID))
	})

	return owning
}

// nodeOf is the node a fake kind's spec names, which is the spec itself.
func nodeOf(spec any) (graph.NodeID, error) {
	id, ok := spec.(graph.NodeID)
	if !ok {
		return "", fmt.Errorf("%w: a fake kind is required as a %T, not as a %T", kind.ErrInvalidPayload, id, spec)
	}

	return id, nil
}
