package deleteResource

import (
	"encoding/json"
	"time"
)

// Request asks for a resource to be deleted.
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

	// Payload is what its kind's delete is asked with, as its codec reads
	// it: a container that runs removed by force. Nothing is the delete asked
	// with nothing.
	Payload json.RawMessage

	// Wait is how long to wait for its node to say it deleted it, or for a
	// root's graph to have removed it: nothing is not waiting.
	Wait time.Duration

	// ByGraph says it is a root's graph removing what it made, the root
	// among it, whose node is asked for its delete as anything's is. Nobody
	// asking through the API says it: a root of a kind whose graph removes it
	// is otherwise marked to be removed, and left to its graph.
	ByGraph bool
}
