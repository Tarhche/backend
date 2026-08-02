package getnotes

type Request struct {
	Page uint
	// AuthorUUID scopes the listing to a single author's own notes. It is empty
	// on the routes guarded by the global notes permissions, which list every
	// author's notes.
	AuthorUUID string
}
