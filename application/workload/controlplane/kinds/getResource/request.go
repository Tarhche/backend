package getResource

// Request is one resource of one kind.
type Request struct {
	// Kind is its kind, by its name.
	Kind string

	// OwnerUUID narrows it to that person's own; empty is anybody's.
	OwnerUUID string
	UUID      string
}
