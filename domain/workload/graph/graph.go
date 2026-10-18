// Package graph is what the workload's resources need of one another, as a
// graph: each kind says what a resource of it needs directly (Kind.Requires),
// and what it needs is made before it, from the bottom up, and removed after
// it, from the top down, through the edges that own it.
//
// A node of a graph is one resource something needs, named by its kind and
// its name (NodeID): a VM by its uuid, a network, a volume or a container by
// its Docker name. That one resource needed by several is one node is what
// makes them share it: one VM for a stack and every container in it, one
// network for every service on it. An edge says one node needs another, and
// its type (EdgeType) what deleting the one does to the other: what is owned
// goes with the last of its owners, what is loose stays, and an order edge
// only orders two nodes that need nothing of each other. Which type an edge
// has is the policy's to say (DependencyPolicy), by the kinds it joins, and
// never a kind's.
//
// Behind every node is its kind (Kind), which says what it needs, whether it
// is there, and makes it and removes it, on the records the control plane
// keeps, through the control plane's own admission, commands and delete.
// Whether it is there is what its node last reported of it, never what a
// node is asked at the time.
//
// Here are the contracts, the plan of what a root needs (Planner, Plan): its
// nodes, each once, what each needs of the others, and the order they are
// made in, a level at a time; what removing the root takes with it, the nodes
// it owns (Plan.Owned), and the order they are removed in (Plan.LevelsOf);
// what owns each node's record and what each uses, as the records say
// (Ownership); how each node of it went the last time it was carried out
// (Execution), which a repository keeps (Executions); which control plane
// carries a root's plan out, and when the root may be carried out again once
// its plan failed (Lease), which a repository keeps too (Leases); which kinds'
// resources are roots of their own graphs, and what each root needs, as its
// record says (RootKind, Roots), and what a restored parent was found to
// hold of a root its restore reset (ResetKind); what is told a record of a
// graph changed, or a root admitted or asked to go, so that the roots are
// carried out again (Notifier, Enqueuer), and what says which roots need a
// record (Dependents), through a Relay where what carries them out is built
// after what tells and asks it. Nothing here carries a plan out: nothing here
// reads or writes anything.
package graph

import (
	"context"
	"strings"

	"github.com/khanzadimahdi/testproject/domain/workload/kind"
)

// NodeID names one node of a graph by its kind and its name, as
// "<kind>/<name>": a VM is "vm/<uuid>", a network "network/<docker name>". It
// is unique within one graph, and two requirements that name the same node
// are the same resource.
type NodeID string

// ID is the node of the named kind and name.
func ID(kindName string, name string) NodeID {
	return NodeID(kindName + "/" + name)
}

// Split is the kind and the name id names a node by, and whether it names
// one at all: both are something, and a kind's name, a word, ends at the
// first slash, so a name may have slashes of its own.
func (id NodeID) Split() (kindName string, name string, ok bool) {
	kindName, name, ok = strings.Cut(string(id), "/")
	if !ok || len(kindName) == 0 || len(name) == 0 {
		return "", "", false
	}

	return kindName, name, true
}

// Kind is the kind of the node id names, or nothing when it names none.
func (id NodeID) Kind() string {
	kindName, _, _ := id.Split()

	return kindName
}

// Name is the name of the node id names, or nothing when it names none.
func (id NodeID) Name() string {
	_, name, _ := id.Split()

	return name
}

func (id NodeID) String() string {
	return string(id)
}

// Ref is the record a node became: what a dependent keeps of it.
type Ref = kind.Reference

// Requirement is one resource another needs directly.
type Requirement struct {
	// Kind is the kind it is of, by its name, and Name what names it among
	// the rest of its kind in a graph: together they are its node (ID), which
	// is what makes two requirements of one resource one node.
	Kind string
	Name string

	// Spec is what its kind needs to know of it, as the kind's own
	// requirement type, which no one else reads.
	Spec any

	// After are nodes it is made after, for nothing but the order: a
	// service and those it depends_on. They need nothing of each other.
	After []NodeID

	// External is a resource that was there before, made by somebody else,
	// which what needs it only names: it is never made or removed through
	// the graph, and what needs it waits while it is missing.
	External bool
}

// ID is the node the requirement is.
func (r Requirement) ID() NodeID {
	return ID(r.Kind, r.Name)
}

// Kind is what a graph needs of a kind: what a resource of it needs, whether
// it is there, and to make it and to remove it. Each is handed the kind's own
// requirement type as spec, and is kind.ErrInvalidPayload for anything else.
type Kind interface {
	// Requires is what a resource needs directly, and nothing more: what
	// that needs in turn is its own kind's to say. It reads nothing but spec.
	Requires(spec any) ([]Requirement, error)

	// Exists is the record a resource is, whenever there is one, and
	// whether its node has made it, as its node last said: a node speaks for
	// what it holds every second, and thirty seconds of silence say it is
	// gone. A record not made yet, or that failed before it was, or that is
	// going away, is its record all the same, and is not made, so that what
	// removes a node finds it whatever it is doing; with no record, it is
	// the zero Ref.
	Exists(ctx context.Context, spec any) (Ref, bool, error)

	// Create makes a resource, and is the record it is once its node has
	// made it: one record however often it is asked, found by what names
	// the node. It is handed the records of the nodes it needs, by their
	// ids. What it needs that is there and cannot take it now is Waiting.
	Create(ctx context.Context, spec any, deps map[NodeID]Ref) (Ref, error)

	// Delete removes the record ref names, and is done once its node has:
	// one gone already is removed. What it lives in that cannot take it now
	// is Waiting.
	Delete(ctx context.Context, ref Ref) error
}

// Holder is a kind some of whose resources may be there with no record of
// theirs, made by somebody else: a network or a volume a Docker VM's dockerd
// holds that was made from its terminal. Holds is the record an external
// node is, as Exists says, or, with none, what holds it with nobody keeping a
// record of it, under the uuid it is seen by, and whether either is there:
// what is external is found where it is, and a graph never makes or removes
// it. A node that is not external is looked for with Exists alone.
type Holder interface {
	Holds(ctx context.Context, spec any) (Ref, bool, error)
}

// Kinds are the kinds a graph is made of, by their names.
type Kinds map[string]Kind

// Waiting is what a kind makes or removes nothing with because what the
// resource needs is there and cannot take it now: a VM that does not run, or
// is being deleted. Its node waits on it rather than failing. errors.As finds
// it, however it is wrapped.
type Waiting struct {
	// Reason is what it waits on, in a few words: "its vm is stopped".
	Reason string
}

func (w *Waiting) Error() string {
	return "waiting: " + w.Reason
}

// Refused is what a kind makes nothing with because its admission refused
// what it was to make, as it would have refused whoever asked for it: no node
// has room for a VM. A root whose plan fails with it says so as its failure,
// by its code, while it is not made, as its own admission would have said it.
// errors.As finds it, however it is wrapped.
type Refused struct {
	// Code is why, in a word the control plane answers a refusal with:
	// no_capacity. Message is what was refused, field by field.
	Code    string
	Message string
}

func (r *Refused) Error() string {
	return r.Message
}
