package getVM

type Request struct {
	UUID string `json:"-"`

	// OwnerUUID narrows what is found to one person's own, and is empty for
	// anybody's: a VM that is not theirs is not there as far as they are
	// concerned.
	OwnerUUID string `json:"-"`
}
