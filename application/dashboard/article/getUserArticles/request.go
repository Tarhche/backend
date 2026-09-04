package getuserarticles

type Request struct {
	Page uint

	// AuthorUUID is whose articles these are. It is not asked for: it is who
	// is asking, filled in by the handler.
	AuthorUUID string
}
