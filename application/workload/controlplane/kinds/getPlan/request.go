package getPlan

// Request asks how a root's plan went.
type Request struct {
	// Kind is the root's kind, by its name.
	Kind string

	// OwnerUUID narrows it to that person's own; empty is anybody's.
	OwnerUUID string

	// UUID names it, and Parent, when it is given, is the resource it lives
	// in, of its kind's parent kind: inside it, a kind may name its
	// resources by more than their uuids (kind.Resolver).
	UUID   string
	Parent string
}
