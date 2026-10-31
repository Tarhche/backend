package getVolumeDependents

type Request struct {
	VMUUID string `json:"-"`

	// Name is the volume's name, which is the only id it has.
	Name string `json:"-"`

	// OwnerUUID narrows what is read to one person's own, and is empty for
	// anybody's.
	OwnerUUID string `json:"-"`
}
