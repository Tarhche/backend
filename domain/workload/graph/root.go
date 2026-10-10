package graph

import (
	"context"

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
