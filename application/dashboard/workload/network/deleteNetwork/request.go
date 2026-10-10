package deleteNetwork

type Request struct {
	VMUUID string `json:"-"`

	// ID is the network's id or its name.
	ID string `json:"-"`

	// OwnerUUID narrows what is acted on to one person's own, and is empty
	// for anybody's.
	OwnerUUID string `json:"-"`

	// Cascade deletes the containers that need it first, nothing owning
	// them: one on it, which would have it refused, or one that would make
	// it again.
	Cascade bool `json:"-"`
}
