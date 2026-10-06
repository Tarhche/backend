package kind

import (
	"context"
	"io"

	"github.com/khanzadimahdi/testproject/domain"
	"github.com/khanzadimahdi/testproject/domain/workload/port"
)

// ControlPlane is the part of a kind the control plane runs.
//
// The control plane's generic code keeps the records, dispatches actions and
// runs the reconcile loop for every kind alike: paging through resources,
// failing those whose node fell silent, deleting those that expired, asking
// again for what is stuck in flight, with backoff. A kind only decides what
// is particular to it.
type ControlPlane[Spec, Status any] interface {
	// Admit makes what somebody asked for into a resource to keep: its
	// defaults, its validation, its owner's quotas and where it is placed.
	// What was asked is its metadata as given, its owner and name and
	// lifetime, and its spec. What is wrong with it comes back field by
	// field, and an error is only what kept it from being looked at.
	Admit(ctx context.Context, asked Resource[Spec, Status]) (Resource[Spec, Status], domain.ValidationErrors, error)

	// Reconcile is what to ask for, given what the resource was asked to be
	// and what it was last observed being. It is not asked about a resource
	// that is in flight, silent or expired: the loop deals with those itself.
	// Nothing to ask for is no intents.
	Reconcile(ctx context.Context, r Resource[Spec, Status]) ([]Intent, error)

	// Apply carries out one of the kind's actions that run in the control
	// plane, renaming a snapshot say, and is the resource as it left it, to be
	// kept. Its payload is what the action's codec decoded: a P for
	// Payload[P], nil for NoPayload.
	Apply(ctx context.Context, r Resource[Spec, Status], action string, payload any) (Resource[Spec, Status], domain.ValidationErrors, error)
}

// Intent is what a kind's reconcile asks for: one of its actions, which the
// loop sends as a Command when it runs on a node and applies in place when it
// runs in the control plane.
type Intent struct {
	Action string

	// Payload is the action's, typed: what its codec would decode.
	Payload any

	// Reason is why it is asked for, for whoever reads the logs.
	Reason string
}

// Node is the part of a kind an orchestrator runs: carrying out its actions
// on the instances this node holds, and saying what they are doing.
//
// It is handed resources as the control plane recorded them, and holds no
// records of its own: what a node knows is what its engine and the dockerds
// of its Docker VMs say.
type Node[Spec, Status any] interface {
	// Execute carries out one of the kind's command actions on r, and is
	// what it left r as. An error is a failure: carried out again, it would
	// fail the same way, so it is said once as the Result and not asked
	// again. The outcome is kept even then, since what compose printed is most
	// worth reading when it failed.
	Execute(ctx context.Context, r Resource[Spec, Status], action string, payload any) (Outcome[Status], error)

	// Query answers one of the kind's query actions about r at once: a log,
	// stats. It is never asked for state, which is answered from State.
	Query(ctx context.Context, r Resource[Spec, Status], action string, payload any) (any, error)

	// State is the kind's state action: every instance of the kind this
	// node holds, as it is now. It is what every heartbeat reports, and what
	// a query for one resource's state is answered from. An error is that it
	// could see nothing; a parent it could not look inside is Unseen, and the
	// rest is reported.
	State(ctx context.Context) (Report[Status], error)
}

// Outcome is what carrying out a command left a resource as.
type Outcome[Status any] struct {
	// Status is the resource's status as the command left it.
	Status Status

	// Output is what the command printed, of which the Result keeps the
	// last MaxOutput bytes.
	Output string
}

// Attacher is a node strategy that also serves a kind's stream actions,
// such as a terminal, on the node's own API, which the ingress carries them
// to. A kind without one has no stream actions.
type Attacher interface {
	// Attach opens a stream action on the instance uuid names, for owner,
	// whose it has to be: the node reads who owns an instance off the
	// instance itself, and one that is not theirs is not there
	// (domain.ErrNotExists), so knowing a uuid says nothing about whether one
	// exists.
	Attach(ctx context.Context, action string, uuid string, owner string) (Session, error)
}

// Session is a stream opened in an instance: a terminal's input and output,
// its size and its end. An engine's vm.ExecSession is one.
type Session interface {
	// Stdin is its input; closing it is the end of the input.
	Stdin() io.WriteCloser

	// Stdout is its output, and under a terminal everything is there.
	Stdout() io.Reader

	// Stderr is its errors, and is empty under a terminal.
	Stderr() io.Reader

	// Resize tells its terminal how big it now is.
	Resize(ctx context.Context, rows uint, cols uint) error

	// Wait waits for it to end and says how it did.
	Wait(ctx context.Context) (exitCode int, err error)

	// Close ends it.
	Close() error
}

// Ingress is the part of a kind the ingress runs: where its instances are
// reached from outside. Only a kind with Endpoints or a stream action has
// one.
//
// Nothing is reached but through the node holding it, so where an instance
// is, is which node holds it. A kind that has nothing by the name or the uuid
// asked for says domain.ErrNotExists, and the ingress asks the next kind; one
// whose instance is there and cannot be reached now, not running or not
// placed yet, says ErrUnreachable, with why.
type Ingress interface {
	// ByUUID is where the instance uuid names is, for a stream to it.
	ByUUID(ctx context.Context, uuid string) (Location, error)

	// BySlug is where the instance a hostname's slug names is, for its
	// ports.
	BySlug(ctx context.Context, slug string) (Location, error)
}

// Location is where an instance is reached through the ingress.
type Location struct {
	UUID string

	// Node is the node holding it, whose tunnel its streams and ports are
	// carried down.
	Node string

	// Ports are the ports it lets the ingress reach: none for one that lets
	// nothing in, whatever it exposes.
	Ports []port.Port
}
