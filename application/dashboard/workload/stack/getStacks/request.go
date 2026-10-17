package getStacks

type Request struct {
	Page uint `json:"page"`

	// VMUUID narrows the listing to the stacks deployed into one VM.
	VMUUID string `json:"vm"`

	// OwnerUUID narrows the listing to one person's own, and is empty for
	// everybody's.
	OwnerUUID string `json:"-"`
}
