package deleteContainer

type Request struct {
	VMUUID string `json:"-"`

	// ID is the container's id or its name.
	ID string `json:"-"`

	// Force removes a container that is still running; without it, one that
	// is running is refused.
	Force bool `json:"force"`

	// OwnerUUID narrows what is acted on to one person's own, and is empty
	// for anybody's.
	OwnerUUID string `json:"-"`
}
