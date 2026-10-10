package disconnectNetwork

type Request struct {
	VMUUID string `json:"-"`

	// ID is the container's id or its name.
	ID string `json:"-"`

	// Network is the network's id or its name.
	Network string `json:"-"`

	OwnerUUID string `json:"-"`
}
