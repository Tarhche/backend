package getStack

type Request struct {
	UUID string `json:"-"`

	// OwnerUUID narrows what is found to one person's own, and is empty for
	// anybody's.
	OwnerUUID string `json:"-"`
}
