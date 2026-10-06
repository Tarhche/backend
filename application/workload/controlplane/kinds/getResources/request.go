package getResources

// Request is a page of the resources of one kind.
type Request struct {
	// Kind is their kind, by its name.
	Kind string

	// OwnerUUID narrows them to that person's own; empty is anybody's.
	OwnerUUID string

	// Parent narrows them to those living in the resource it names, of the
	// kind's parent kind: a Docker VM's building blocks.
	Parent string

	Page uint
}
