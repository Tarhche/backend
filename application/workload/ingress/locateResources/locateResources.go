// Package locateResources hears where the tasks and the VMs are, and writes
// it down where the ingress looks for them (ingress.Locations): the ingress
// reads no records.
//
// What it hears is what the nodes holding them say, and what the control
// plane sends those nodes, over core NATS rather than through a JetStream
// consumer: every ingress hears all of it as it is said, and none of it is
// kept for one that is not listening, which starts knowing where everything
// is a beat later.
//
//   - A heartbeat (kind.HeartbeatName, the kind's own subject) says one
//     instance its node holds: the slug it is reached under, what it is doing
//     and the ports it lets the ingress reach. Every node says every instance
//     it holds once a beat, and nothing of what it does not: one gone unheard
//     for long enough is forgotten where it was written down, not here.
//   - A command (kind.ActOnResourceName), sent to the node holding a resource,
//     takes away at once what carrying it out will, before its node has: all
//     of it, when its action leaves the resource anything but running, a stop
//     or a delete say, as its kind's descriptor says; and otherwise whatever
//     of its ports the resource, as the command carries it, no longer lets in.
//     A command never gives anything, and what it took away comes back with
//     nothing its node says until it is answered, or has gone unanswered for
//     as long as a resource may go unheard.
//   - What came of a command (kind.ResourceActedOnName) answers it: carried
//     out, where it left the resource is taken as a heartbeat is, at once,
//     and a resource it deleted is gone. One that failed, was refused or says
//     nothing changes nothing.
//
// Of what a node says of a resource, nothing said before what was last taken
// from that node is taken. It hears the kinds the ingress routes to, the
// tasks and the VMs, each read by its own ingress strategy, and no other.
//
// Nothing heard is failed: the next beat says it all again. What cannot be
// read, or names no node, no kind or no resource, is let go of.
package locateResources

import (
	"encoding/json"

	"github.com/khanzadimahdi/testproject/domain/workload/ingress"
	"github.com/khanzadimahdi/testproject/domain/workload/kind"
	"github.com/khanzadimahdi/testproject/domain/workload/port"
)

// Kind is a kind the ingress routes to, as its ingress strategy knows it:
// what it is, the state its resources are reached in, where what is said of
// one of them says it is, and what a command carrying one lets in.
type Kind interface {
	// Descriptor is the kind, whose actions say what each command desires.
	Descriptor() kind.Descriptor

	// Running is the state its resources are reached in, and the only one.
	Running() kind.State

	// Read is where a resource of the kind is, as its status says, in a
	// heartbeat or in what came of a command: the slug it is reached under,
	// what it is doing and the ports it lets the ingress reach. Which
	// resource it is, the node holding it and when it was said are the
	// message's.
	Read(status json.RawMessage) (ingress.Heard, error)

	// Allowed are the ports a resource lets the ingress reach as the control
	// plane recorded it, which is how a command carries it: the most a
	// command leaves it reached on.
	Allowed(r kind.Raw) ([]port.Port, error)
}
