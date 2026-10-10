// Package graphtest is a graph.Kind a test says everything of (Kind), which
// may stand for every kind of a graph at once: what the tests of a plan, and
// of what carries one out, are written against.
package graphtest

import (
	"context"
	"fmt"
	"maps"
	"slices"
	"sync"

	"github.com/khanzadimahdi/testproject/domain/workload/graph"
	"github.com/khanzadimahdi/testproject/domain/workload/kind"
)

// Op is what a kind is asked of a resource.
type Op string

const (
	// Requires is asking what it needs, Exists whether it is there, Create
	// to make it and Delete to remove it.
	Requires Op = "requires"
	Exists   Op = "exists"
	Create   Op = "create"
	Delete   Op = "delete"
)

// Call is a kind asked something of a node: Deps are the records of what it
// needs that a create was handed.
type Call struct {
	Op   Op
	Node graph.NodeID
	Deps map[graph.NodeID]graph.Ref
}

// Event is a call to a kind beginning, or, when Returned, returning.
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
// What a node requires is what Require said, and nothing else. Whether it is
// there is whether the kind holds it: Create holds it, Delete lets it go, and
// Hold holds it as if it were made before; one that is not held is removed
// all the same. What a test has a node do when it is asked something (On) is
// done first, and what that fails with is what it is answered. Everything the
// kind is asked is recorded (Calls), and when each call began and returned
// (Events), and it may be asked from several goroutines at once.
type Kind struct {
	lock     sync.Mutex
	requires map[graph.NodeID][]graph.Requirement
	held     map[graph.NodeID]bool
	on       map[Op]map[graph.NodeID]func(ctx context.Context) error
	calls    []Call
	events   []Event
}

var _ graph.Kind = &Kind{}

// NewKind is a kind that holds nothing, and whose every node requires
// nothing.
func NewKind() *Kind {
	return &Kind{
		requires: make(map[graph.NodeID][]graph.Requirement),
		held:     make(map[graph.NodeID]bool),
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
		k.held[id] = true
	}
}

// Held are the nodes the kind holds, in the order of their ids.
func (k *Kind) Held() []graph.NodeID {
	k.lock.Lock()
	defer k.lock.Unlock()

	return slices.Sorted(maps.Keys(k.held))
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

// Calls are everything the kind was asked, in order.
func (k *Kind) Calls() []Call {
	k.lock.Lock()
	defer k.lock.Unlock()

	calls := make([]Call, 0, len(k.calls))
	for _, c := range k.calls {
		c.Deps = maps.Clone(c.Deps)
		calls = append(calls, c)
	}

	return calls
}

// Events are when each call to the kind began, and when it returned, in the
// order they did: a call returns once what it does is done, so whatever began
// after it returned began after all of it.
func (k *Kind) Events() []Event {
	k.lock.Lock()
	defer k.lock.Unlock()

	return slices.Clone(k.events)
}

// Asked are the nodes the kind was asked op of, in order, once each time.
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

	if err := k.ask(context.Background(), Requires, id, nil); err != nil {
		return nil, err
	}

	k.lock.Lock()
	defer k.lock.Unlock()

	return slices.Clone(k.requires[id]), nil
}

// Exists is the record of the node spec names, when the kind holds it.
func (k *Kind) Exists(ctx context.Context, spec any) (graph.Ref, bool, error) {
	id, err := nodeOf(spec)
	if err != nil {
		return graph.Ref{}, false, err
	}

	defer k.returned(Exists, id)

	if err := k.ask(ctx, Exists, id, nil); err != nil {
		return graph.Ref{}, false, err
	}

	k.lock.Lock()
	defer k.lock.Unlock()

	if !k.held[id] {
		return graph.Ref{}, false, nil
	}

	return Ref(id), true, nil
}

// Create holds the node spec names, and is its record.
func (k *Kind) Create(ctx context.Context, spec any, deps map[graph.NodeID]graph.Ref) (graph.Ref, error) {
	id, err := nodeOf(spec)
	if err != nil {
		return graph.Ref{}, err
	}

	defer k.returned(Create, id)

	if err := k.ask(ctx, Create, id, deps); err != nil {
		return graph.Ref{}, err
	}

	k.lock.Lock()
	defer k.lock.Unlock()

	k.held[id] = true

	return Ref(id), nil
}

// Delete lets the node ref is the record of go.
func (k *Kind) Delete(ctx context.Context, ref graph.Ref) error {
	id := graph.ID(ref.Kind, ref.UUID)

	defer k.returned(Delete, id)

	if err := k.ask(ctx, Delete, id, nil); err != nil {
		return err
	}

	k.lock.Lock()
	defer k.lock.Unlock()

	delete(k.held, id)

	return nil
}

// ask records the node id asked op, and does what the test has it do first,
// outside the lock, so that it may block.
func (k *Kind) ask(ctx context.Context, op Op, id graph.NodeID, deps map[graph.NodeID]graph.Ref) error {
	k.lock.Lock()
	k.calls = append(k.calls, Call{Op: op, Node: id, Deps: maps.Clone(deps)})
	k.events = append(k.events, Event{Op: op, Node: id})
	do := k.on[op][id]
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

// nodeOf is the node a fake kind's spec names, which is the spec itself.
func nodeOf(spec any) (graph.NodeID, error) {
	id, ok := spec.(graph.NodeID)
	if !ok {
		return "", fmt.Errorf("%w: a fake kind is required as a %T, not as a %T", kind.ErrInvalidPayload, id, spec)
	}

	return id, nil
}
