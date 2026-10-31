package getSnapshots

type Request struct {
	Page uint `json:"page"`

	// VMUUID narrows the listing to the snapshots of one VM.
	VMUUID string `json:"vm"`

	// OwnerUUID narrows the listing to one person's own, and is empty for
	// everybody's.
	OwnerUUID string `json:"-"`
}
