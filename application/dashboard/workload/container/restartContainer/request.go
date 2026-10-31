package restartContainer

type Request struct {
	VMUUID string `json:"-"`

	// ID is the container's id or its name.
	ID string `json:"-"`

	// OwnerUUID narrows what is acted on to one person's own, and is empty
	// for anybody's.
	OwnerUUID string `json:"-"`
}
