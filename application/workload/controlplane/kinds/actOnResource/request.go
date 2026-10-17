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

	// UUID names it, and Parent, when it is given, is the resource it lives
	// in, of its kind's parent kind: what lives elsewhere is not there. Inside
	// a parent, a kind may name its resources by more than their uuids, as a
	// container is named by its Docker id or its name (kind.Resolver).
	UUID   string
	Parent string

	Action  string
	Payload json.RawMessage

	// Wait is how long to wait for what came of the command, when it is one
	// for a node: nothing is not waiting.
	Wait time.Duration

	// Cascade deletes the roots whose graphs need what a command that
	// deletes it is asked of, with it, as a delete with them does
	// (deleteResource.Request.Cascade): each marked to go first, and those
	// that do not live in it waited on to go before it is asked. It says
	// nothing to any other command, nor to a root's own delete.
	Cascade bool
}
