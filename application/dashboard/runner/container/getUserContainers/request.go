package getusercontainers

type Request struct {
	Page uint `json:"page"`

	// OwnerUUID is whose containers these are. It is not asked for: it is who is
	// asking, filled in by the handler.
	OwnerUUID string `json:"-"`
}
