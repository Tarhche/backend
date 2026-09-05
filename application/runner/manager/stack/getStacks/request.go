package getStacks

const Limit uint = 10

type Request struct {
	Page uint `json:"page"`

	// OwnerUUID narrows the listing to one person's own stacks. Empty is
	// everybody's.
	OwnerUUID string `json:"owner_uuid,omitempty"`
}
