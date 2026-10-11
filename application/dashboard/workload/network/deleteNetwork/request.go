package deleteNetwork

type Request struct {
	VMUUID string `json:"-"`

	// ID is the network's id or its name.
	ID string `json:"-"`

	// OwnerUUID narrows what is acted on to one person's own, and is empty
	// for anybody's.
	OwnerUUID string `json:"-"`

	// Cascade deletes what needs it first, the stacks and the containers
	// nothing owns: a container on it, which would have it refused, or what
	// would make it again.
	Cascade bool `json:"-"`
}
