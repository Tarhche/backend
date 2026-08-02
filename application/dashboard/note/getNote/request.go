package getnote

type Request struct {
	CorrelationUUID string
	LanguageCode    string
	// OwnerUUID scopes the lookup to a single author's own notes. It is empty on
	// the routes guarded by the global notes permissions.
	OwnerUUID string
}
