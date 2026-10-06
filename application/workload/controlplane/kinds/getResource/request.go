package getResource

// Request is one resource of one kind.
type Request struct {
	// Kind is its kind, by its name.
	Kind string

	// OwnerUUID narrows it to that person's own; empty is anybody's.
	OwnerUUID string

	// UUID names it, and Parent, when it is given, is the resource it lives
	// in, of its kind's parent kind: what lives elsewhere is not there. Inside
	// a parent, a kind may name its resources by more than their uuids, as a
	// container is named by its Docker id or its name (kind.Resolver).
	UUID   string
	Parent string
}
