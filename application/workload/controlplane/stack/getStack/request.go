package getStack

// Request names a stack, as one person's own when OwnerUUID is set.
type Request struct {
	OwnerUUID string
	UUID      string
}
