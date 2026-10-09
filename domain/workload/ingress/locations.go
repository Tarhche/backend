package ingress

import (
	"context"
	"time"

	"github.com/khanzadimahdi/testproject/domain/workload/kind"
	"github.com/khanzadimahdi/testproject/domain/workload/port"
)

// Locations are where the resources the ingress routes to are, a task's or
// a VM's, as the nodes holding them last said, and as the commands sent to
// those nodes left them: by kind, and by uuid or by slug.
//
// They are what was said, not records. Every node says every instance it
// holds once a beat, so an ingress that has just started knows where
// everything is a beat later, and nothing of a resource no node holds: one
// admitted and not placed yet, or one its node has not made yet, is not
// there. No heartbeat says a resource is gone, so one that has gone unheard
// for long enough is forgotten.
//
// Access only ever comes from what a node says, in a heartbeat or in what
// came of a command; a command only ever takes it away, at once, before its
// node has carried it out, and what it took away comes back with nothing its
// node says until it is answered.
//
// Heartbeats, commands and their results are heard while requests are being
// routed, so an implementation has to be safe for concurrent use.
type Locations interface {
	// Hear writes down where a resource is, as a node's heartbeat said, in
	// place of what was heard of it before: unless the same node said
	// something of it later already, which a heartbeat heard late does not
	// undo, or a command withholds it. Of two nodes that both speak for one
	// resource, the one heard last says where it is. It fails nothing: the
	// next beat says it all again.
	Hear(ctx context.Context, heard Heard)

	// Withhold takes away at once what a command sent to a resource's node
	// takes away of it, until its answer is heard: all of it, when the
	// command leaves it out of being reached, and otherwise whatever of its
	// ports the command no longer lets in. It never gives anything.
	Withhold(ctx context.Context, withheld Withheld)

	// Answer writes down what came of a command: what it withheld comes
	// back, and where its result says it left the resource is heard as Hear
	// hears it.
	Answer(ctx context.Context, answered Answered)

	// ByUUID is what was last heard of the resource of the named kind uuid
	// names. One nothing has been heard of lately is domain.ErrNotExists; any
	// other error is this being unable to say, which locations kept anywhere
	// but in memory can be, and which is not the same answer at all.
	ByUUID(ctx context.Context, kindName string, uuid string) (Heard, error)

	// BySlug is what was last heard of the resource of the named kind a slug
	// names, as ByUUID is.
	BySlug(ctx context.Context, kindName string, slug string) (Heard, error)
}

// Heard is what a node said of where one resource is: what is needed to
// route to it, and when.
type Heard struct {
	Kind string
	UUID string

	// Slug is the name its ports are served under, as its node was given it.
	// One heard under none is found under the slug it was heard under
	// before, and by its uuid alone when it never was.
	Slug string

	// Node is the node holding it, whose tunnel its streams and ports are
	// carried down.
	Node string

	// State is what it is doing, as its node said.
	State kind.State

	// Ports are the ports it lets the ingress reach, as its node said: none
	// for one that lets nothing in, whatever it exposes.
	Ports []port.Port

	// At is when its node said it, by the node's clock: when the beat that
	// said it was taken, or when the command it answers was carried out.
	At time.Time

	// Gone says it is nowhere any more: its node said a command deleted it.
	Gone bool
}

// Withheld is what a command sent to the node holding a resource takes away
// of it, as soon as the command is sent.
type Withheld struct {
	Kind string
	UUID string

	// Command is the command's ID, whose answer gives back what it withheld.
	Command string

	// State is what the command leaves the resource doing when that takes it
	// out of being reached, a stop's or a delete's, and nothing when it
	// leaves it reached.
	State kind.State

	// Ports are the ports the resource lets the ingress reach as the command
	// carries it: of the ports it was heard to let in, only those both allow
	// are left.
	Ports []port.Port
}

// Answered is what came of a command sent to the node holding a resource.
type Answered struct {
	Kind string
	UUID string

	// Command is the ID of the command answered.
	Command string

	// Heard is where the command left the resource, as its result said, and
	// nothing when its result says nothing to be taken: the command failed,
	// was refused, or left nothing said.
	Heard *Heard
}
