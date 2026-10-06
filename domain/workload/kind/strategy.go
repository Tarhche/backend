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

// Preparer is a control-plane strategy that readies its kind's node commands
// before they are sent, with what only the control plane knows: a VM's restore
// is held to a snapshot of its owner's that is ready and fits, and a VM no
// node had room for is placed on one before it is started.
//
// It is handed the resource as it is recorded and the command's payload, as
// the action's codec decoded it, and is the resource the command carries and
// is written down as: unchanged, placed on a node, or given what only the
// control plane could give it. What is wrong with the command comes back
// field by field, and nothing is asked then. A strategy that has nothing to
// ready for an action hands the resource back as it was.
type Preparer[Spec, Status any] interface {
	Prepare(ctx context.Context, r Resource[Spec, Status], action string, payload any) (Resource[Spec, Status], domain.ValidationErrors, error)
}

// Extras are resources a kind's listings show beside its own records, which
// nothing keeps as its records: the code runner's runs, which are its tasks,
// shown among anybody's VMs. None of them is anybody's own, so only a listing
// of anybody's has them; and what they can be asked is theirs to say,
// whatever they cannot be asked being refused field by field.
//
// The control plane's generic API asks them for what a uuid names when none
// of the kind's records is it.
type Extras interface {
	// All is every extra there is now, as manifests of the kind, newest
	// first.
	All(ctx context.Context) ([]Raw, error)

	// One is the extra uuid names, or domain.ErrNotExists.
	One(ctx context.Context, uuid string) (Raw, error)

	// Act asks an extra for one of its kind's commands, with its payload as
	// it was given, and is the extra as the command left it, or gone when the
	// command took it away.
	Act(ctx context.Context, r Raw, action string, payload []byte) (after Raw, gone bool, refused domain.ValidationErrors, err error)

	// Query asks an extra one of its kind's queries, and is the answer as
	// its node strategy would give it.
	Query(ctx context.Context, r Raw, action string, payload []byte) (answer []byte, refused domain.ValidationErrors, err error)
}

// Extender is a control-plane strategy whose kind's listings show Extras
// beside its records.
type Extender interface {
	Extras() Extras
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

// Exposer is a node strategy that also serves its kind's ports: where, on
// this node, a port of the instance a slug names is reached, which the
// ingress carries the instance's own traffic to. A kind with Endpoints has
// one, since nothing but the node holding an instance can see where its
// ports are.
type Exposer interface {
	// Endpoint is where port p of the instance slug names is reached from
	// this node, or the lowest port it exposes when p is zero. A slug the
	// node holds nothing by, and a port the instance does not expose, are
	// domain.ErrNotExists; an instance that is here and cannot be reached
	// now, one that is not running, is ErrUnreachable, with why.
	Endpoint(ctx context.Context, slug string, p port.Port) (Endpoint, error)
}

// Endpoint is where one port of an instance is reached on the node holding
// it.
type Endpoint struct {
	// Port is the instance's own port that answers: the one asked for, or
	// the lowest it exposes when none was.
	Port port.Port

	// Address is the host:port a request to it is sent to.
	Address string
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
