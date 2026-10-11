package getNetworkDependents

type Request struct {
	VMUUID string `json:"-"`

	// ID is the network's id or its name.
	ID string `json:"-"`

	// OwnerUUID narrows what is read to one person's own, and is empty for
	// anybody's.
	OwnerUUID string `json:"-"`
}
