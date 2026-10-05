package getSnapshot

// Request names a snapshot, as one person's own when OwnerUUID is set.
type Request struct {
	OwnerUUID string
	UUID      string
}
