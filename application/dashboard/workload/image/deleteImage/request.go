package deleteImage

type Request struct {
	VMUUID string `json:"-"`

	// ID is the image's id or one of its tags.
	ID string `json:"-"`

	// Force removes it even while a container uses it.
	Force bool `json:"force"`

	// OwnerUUID narrows what is acted on to one person's own, and is empty
	// for anybody's.
	OwnerUUID string `json:"-"`
}
