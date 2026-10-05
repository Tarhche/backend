package getSnapshots

// Request is a page of snapshots: one person's own when OwnerUUID is set, and
// only those taken of one VM when VMUUID is.
type Request struct {
	OwnerUUID string
	VMUUID    string
	Page      uint
}
