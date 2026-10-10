package deleteStack

type Request struct {
	UUID string `json:"-"`

	// RemoveVolumes takes the stack's volumes with it. They are kept
	// otherwise, since what is in them is usually what somebody wants back.
	RemoveVolumes bool `json:"volumes"`

	// OwnerUUID narrows what is acted on to one person's own, and is empty
	// for anybody's.
	OwnerUUID string `json:"-"`
}
