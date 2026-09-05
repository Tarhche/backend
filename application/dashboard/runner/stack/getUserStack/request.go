package getuserstack

// Request is one of somebody's own stacks to show.
type Request struct {
	UUID string `json:"-"`

	// OwnerUUID is whose stack this has to be. It is not asked for: it is who
	// is asking, filled in by the handler.
	OwnerUUID string `json:"-"`
}
