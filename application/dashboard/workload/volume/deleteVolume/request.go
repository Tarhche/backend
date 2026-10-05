package deleteVolume

type Request struct {
	VMUUID string `json:"-"`

	// Name is the volume's name, which is the only id it has.
	Name string `json:"-"`

	// Force removes it even while a container uses it.
	Force bool `json:"force"`

	// OwnerUUID narrows what is acted on to one person's own, and is empty
	// for anybody's.
	OwnerUUID string `json:"-"`
}
