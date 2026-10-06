package queryResource

import "encoding/json"

// Request asks a resource one of its kind's queries.
type Request struct {
	// Kind is the resource's kind, by its name.
	Kind string

	// OwnerUUID narrows it to that person's own; empty is anybody's.
	OwnerUUID string
	UUID      string

	Action  string
	Payload json.RawMessage
}
