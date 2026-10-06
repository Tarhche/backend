package deleteResource

import "time"

// Request asks for a resource to be deleted.
type Request struct {
	// Kind is the resource's kind, by its name.
	Kind string

	// OwnerUUID narrows it to that person's own; empty is anybody's.
	OwnerUUID string
	UUID      string

	// Wait is how long to wait for its node to say it deleted it: nothing is
	// not waiting.
	Wait time.Duration
}
