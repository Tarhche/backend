package actOnResource

import (
	"encoding/json"
	"time"
)

// Request asks a resource for one of its kind's commands.
type Request struct {
	// Kind is the resource's kind, by its name.
	Kind string

	// OwnerUUID narrows it to that person's own; empty is anybody's.
	OwnerUUID string
	UUID      string

	Action  string
	Payload json.RawMessage

	// Wait is how long to wait for what came of the command, when it is one
	// for a node: nothing is not waiting.
	Wait time.Duration
}
