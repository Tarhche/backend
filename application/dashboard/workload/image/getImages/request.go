package getImages

type Request struct {
	VMUUID string `json:"-"`

	// OwnerUUID narrows what is found to one person's own, and is empty for
	// anybody's.
	OwnerUUID string `json:"-"`
}
