package getDependents

// Request asks which roots need a resource.
type Request struct {
	// Kind is the resource's kind, by its name.
	Kind string

	// OwnerUUID narrows it to that person's own; empty is anybody's.
	OwnerUUID string

	// UUID names it, and Parent, when it is given, is the resource it lives
	// in, of its kind's parent kind: what lives elsewhere is not there. Inside
	// a parent, a kind may name its resources by more than their uuids, as a
	// network is named by its Docker id or its name in its Docker VM
	// (kind.Resolver).
	UUID   string
	Parent string
}
