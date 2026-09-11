package restartusercontainer

type Request struct {
	UUID string `json:"-"`

	// OwnerUUID is whose container this has to be. It is not asked for: it is who
	// is asking, filled in by the handler.
	OwnerUUID string `json:"-"`
}
