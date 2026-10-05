package getStacks

// Request is a page of stacks: one person's own when OwnerUUID is set, and
// only those in one VM when VMUUID is.
type Request struct {
	OwnerUUID string
	VMUUID    string
	Page      uint
}
