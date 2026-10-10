package graph

import "context"

// Ownership is what owns the record of each node, and what each uses, as the
// records keep it: what a node owns through an owned edge names it as one of
// its owners (its metadata's owners), and what a node uses through a loose
// edge is one of its refs, as the resource it lives in always is. What owns a
// record keeps it while what is removed with a root goes. It is the records
// that say it, not a plan, so that what owns a record is known whatever a
// plan says now.
type Ownership interface {
	// Owners are the records that own ref's record and are still there: one
	// whose record is gone owns nothing, so that an owner left dangling
	// keeps nothing. The resource a record lives in never owns it, whatever
	// a record kept before refs says. A record that is gone is owned by
	// nothing.
	Owners(ctx context.Context, ref Ref) ([]Ref, error)

	// Disown takes owners out of what owns ref's record, and is what owns it
	// still, as Owners says it. A record that is gone is disowned already,
	// and owned by nothing.
	Disown(ctx context.Context, ref Ref, owners ...Ref) ([]Ref, error)

	// Link keeps the records of the two ends of an edge as its type says,
	// whatever they said before, so that linking again changes nothing:
	//
	//   - owned: dependent owns dependency, among its owners, and dependency
	//     is not among what dependent uses, its refs, unless it is the
	//     resource dependent lives in, which stays there;
	//   - loose: dependent uses dependency, among its refs, and does not own
	//     it, among its owners;
	//   - order: nothing, as an order holds nothing of what it orders.
	//
	// A record that is gone is linked to nothing.
	Link(ctx context.Context, dependency Ref, dependent Ref, t EdgeType) error

	// Forget removes root's record, once everything its plan removes is
	// removed: its kind's Delete may leave it, so that it goes last. A root
	// that is gone is forgotten already.
	Forget(ctx context.Context, root Ref) error
}
