package graph

import "context"

// Ownership is what owns the record of each node, as the records keep it:
// what a node owns through an owned edge names it as one of its owners (its
// metadata's owners), and what owns a record keeps it while what is removed
// with a root goes. It is the records that say it, not a plan, so that what
// owns a record is known whatever a plan says now.
type Ownership interface {
	// Owners are the records that own ref's record and are still there: one
	// whose record is gone owns nothing, so that an owner left dangling
	// keeps nothing. A record that is gone is owned by nothing.
	Owners(ctx context.Context, ref Ref) ([]Ref, error)

	// Disown takes owners out of what owns ref's record, and is what owns it
	// still, as Owners says it. A record that is gone is disowned already,
	// and owned by nothing.
	Disown(ctx context.Context, ref Ref, owners ...Ref) ([]Ref, error)

	// Forget removes root's record, once everything its plan removes is
	// removed: its kind's Delete may leave it, so that it goes last. A root
	// that is gone is forgotten already.
	Forget(ctx context.Context, root Ref) error
}
