package planResource

import "github.com/khanzadimahdi/testproject/domain/workload/kind"

// Request asks what admitting a resource, or deleting one, would do, as a
// dry run: nothing is done.
type Request struct {
	// Kind is the resource's kind, by its name.
	Kind string

	// OwnerUUID is whom a resource to admit is for, and narrows one to delete
	// to that person's own; empty is anybody's.
	OwnerUUID string

	// Parent is the resource it is to live in, or lives in, of its kind's
	// parent kind: inside it, a kind may name what is deleted by more than
	// its uuid (kind.Resolver).
	Parent string

	// Manifest is what would be admitted, as admitResource is asked it.
	Manifest kind.Raw

	// Delete asks what deleting the resource UUID names would do, rather
	// than what admitting Manifest would; and Cascade, what deleting it with
	// the roots whose graphs need it would (deleteResource.Request.Cascade).
	Delete  bool
	UUID    string
	Cascade bool
}
