package getuserarticle

type Request struct {
	CorrelationUUID string
	LanguageCode    string

	// AuthorUUID is whose article this has to be. It is not asked for: it is
	// who is asking, filled in by the handler.
	AuthorUUID string
}
