package deleteVM

type Request struct {
	UUID string `json:"-"`

	// OwnerUUID narrows what is acted on to one person's own, and is empty
	// for anybody's.
	OwnerUUID string `json:"-"`

	// Cascade deletes the containers in it with it, which would otherwise
	// make it again, under its uuid and as it was, and then themselves in it.
	Cascade bool `json:"-"`
}
