package getVM

// Request names a VM, as one person's own when OwnerUUID is set.
type Request struct {
	OwnerUUID string
	UUID      string
}
