// Package stack is the stack kind: a compose project deployed into a Docker
// VM, declared once for every service that runs it (domain/workload/kind).
//
// A stack is a composite kind. What it is made of are the building blocks of
// the Docker VM it lives in, which its node strategy uses to reach what its
// spec says:
//
//   - the Docker VM itself, its parent, which has to be running: a stack in
//     one that is not waits on it, and says why, rather than failing;
//   - that VM's dockerd, whose containers carrying the stack's label
//     (LabelStack) are what the stack is now;
//   - and an Applier, which brings those containers to what the stack's
//     compose file says: compose itself, run inside the VM.
//
// Its state is those containers mapped onto the compose file's services, in
// one word: running when every service runs, degraded when some do and some
// do not, stopped when none does, and waiting while its VM is not running or
// the stack is not in it. The control plane never looks at the containers: it
// asks the stack, and brings a degraded one back by applying it again.
//
// A stack's spec is immutable: there is no update, only a new stack.
package stack

import (
	"context"

	"github.com/khanzadimahdi/testproject/domain"
	"github.com/khanzadimahdi/testproject/domain/workload/kind"
	"github.com/khanzadimahdi/testproject/domain/workload/port"
)

const (
	// Name is the kind's word, and Plural what its routes and its
	// permissions are named by: workload.stacks.<verb>.
	Name   = "stack"
	Plural = "stacks"

	// Parent is the kind a stack lives in: a VM, which is always a Docker VM.
	Parent = "vm"
)

// A stack's states. Waiting, Failed and Deleted are the framework's own.
const (
	// Deploying is a stack being brought up as its compose file says: made in
	// its VM, or made again.
	Deploying kind.State = "deploying"

	// Running is a stack whose every service runs, and Degraded one some of
	// whose services do not.
	Running  kind.State = "running"
	Degraded kind.State = "degraded"

	Starting   kind.State = "starting"
	Stopping   kind.State = "stopping"
	Stopped    kind.State = "stopped"
	Restarting kind.State = "restarting"

	// Removing is a stack being taken down. Its record goes once it is.
	Removing kind.State = "removing"

	// Waiting is a stack that is not in its VM: one whose VM is not running,
	// which waits on it, and one that is not deployed there, yet or any
	// more. Its reason says which.
	Waiting = kind.Waiting

	// Failed is a stack whose last command failed. Its reason and its output
	// say why.
	Failed = kind.Failed

	Deleted = kind.Deleted
)

// A stack's actions.
const (
	// ActionCreate deploys a stack that is not in its VM; ActionApply
	// deploys one again, to bring back what it lost. Both are the workload's
	// own to ask for, and are compose's up.
	ActionCreate = "create"
	ActionApply  = "apply"

	ActionStart   = "start"
	ActionStop    = "stop"
	ActionRestart = "restart"

	// ActionDelete takes a stack down, its volumes with it when its payload
	// says so (DeletePayload).
	ActionDelete = "delete"

	// ActionState is what a stack is doing, as its VM's dockerd says.
	ActionState = "state"
)

// The labels the platform puts on every container of a stack, through every
// service of its compose file, so that what is a stack's is known exactly
// rather than guessed from a name.
const (
	// LabelStack is the uuid of the stack a container belongs to.
	LabelStack = "workload.stack"

	// LabelServices are the stack's services that are to be running, comma
	// separated: what a node maps a stack's containers onto, since it keeps
	// no record of the compose file.
	LabelServices = "workload.stack.services"
)

// Spec is what a stack is asked for as: the Docker VM it goes into and its
// compose file, neither of which ever changes.
type Spec struct {
	VM VMChoice `json:"vm"`

	// Compose is the YAML as it was given. What it calls its project is
	// ignored: the stack's slug is its project.
	Compose string `json:"compose"`
}

// VMChoice is which Docker VM a stack goes into.
//
// As it is asked, it names one of the person's Docker VMs by its uuid, or
// names none, which is a Docker VM made for the stack with whatever of the
// rest it gives and the defaults for what it leaves out. As it is kept, it is
// the uuid of the VM the stack went into, made for it or not, and nothing
// else: what a VM was made with is its own record's to say.
type VMChoice struct {
	UUID string `json:"uuid,omitempty"`

	// Name, Resources, Ports and Network are what a Docker VM made for a
	// stack is given beyond the defaults. Anything left out is the default;
	// ports that are there and empty are none.
	Name      string      `json:"name,omitempty"`
	Resources *Resources  `json:"resources,omitempty"`
	Ports     []port.Port `json:"ports,omitzero"`
	Network   *Network    `json:"network,omitempty"`
}

// Describes reports whether a choice describes a VM to make: whether it gives
// any of what one is made with beyond the defaults.
func (c VMChoice) Describes() bool {
	return len(c.Name) > 0 || c.Resources != nil || c.Ports != nil || c.Network != nil
}

// Resources are a VM's sizes: whole vCPUs, and bytes. A size of zero is the
// default one.
type Resources struct {
	CPUs   uint   `json:"cpus"`
	Memory uint64 `json:"memory"`
	Disk   uint64 `json:"disk"`
}

// Network is which ways a VM's network is open, allow or deny. A way left
// empty is the default one.
type Network struct {
	Ingress string `json:"ingress,omitempty"`
	Egress  string `json:"egress,omitempty"`
}

// Status is what a stack is doing: its state, and its services as its VM's
// dockerd has them.
type Status struct {
	kind.Status

	// Services are the stack's services and their containers, as they were
	// last seen.
	Services []ServiceStatus `json:"services,omitempty"`

	// Output is what the last command run on the stack printed, its last
	// kind.MaxOutput bytes: the end of it is what says why a service did not
	// come up.
	Output string `json:"output,omitempty"`
}

// ServiceState is what one service of a stack is doing.
type ServiceState string

const (
	// ServiceRunning is a service whose every container runs.
	ServiceRunning ServiceState = "running"

	// ServiceDegraded is a service some of whose containers run, of several.
	ServiceDegraded ServiceState = "degraded"

	// ServiceStopped is a service none of whose containers runs.
	ServiceStopped ServiceState = "stopped"

	// ServiceMissing is a service that is to be running and has no container.
	ServiceMissing ServiceState = "missing"

	// ServiceCompleted is a service that runs to its end, one another service
	// waits on to have completed, and has: its container exited with 0.
	ServiceCompleted ServiceState = "completed"
)

// ServiceStatus is one service of a stack.
type ServiceStatus struct {
	Name       string            `json:"name"`
	State      ServiceState      `json:"state"`
	Containers []ContainerStatus `json:"containers,omitempty"`
}

// ContainerStatus is one container of a stack's service. Its state is
// docker's word: created, running, paused, restarting, removing, exited or
// dead.
type ContainerStatus struct {
	ID    string `json:"id"`
	Name  string `json:"name"`
	State string `json:"state"`
}

// DeletePayload is what a stack is deleted with.
type DeletePayload struct {
	// RemoveVolumes takes the stack's volumes away with it. They are kept
	// otherwise, since what is in them is usually what somebody wants back.
	RemoveVolumes bool `json:"remove_volumes,omitempty"`
}

var _ domain.Validatable = &DeletePayload{}

// Validate says nothing is wrong: either way is a delete.
func (p *DeletePayload) Validate() domain.ValidationErrors {
	return nil
}

// Stack is a stack, as its strategies are handed one.
type Stack = kind.Resource[Spec, Status]

// VMOf is the uuid of the Docker VM a stack lives in: its parent.
func VMOf(s Stack) string {
	if parent, ok := s.Metadata.Owner(Parent); ok {
		return parent.UUID
	}

	return s.Spec.VM.UUID
}

// Applier brings the containers of a stack to what is asked of them, inside
// the Docker VM the stack lives in.
//
// It is a strategy: compose, run inside the VM, is its one implementation,
// because it keeps all of compose's own behaviour, its dependencies,
// healthchecks, profiles, networks and volumes; one that makes the containers
// one by one could be another, and nothing else would change. Each call
// answers with what it printed, which is all there is to say about a stack
// that did not come up the way it was written, and labels what it makes as
// the stack's (LabelStack, LabelServices).
type Applier interface {
	// Up makes what the stack's compose file has and is not there, starts
	// it, and takes away what the file no longer has.
	Up(ctx context.Context, s Stack) (output string, err error)

	Start(ctx context.Context, s Stack) (output string, err error)
	Stop(ctx context.Context, s Stack) (output string, err error)
	Restart(ctx context.Context, s Stack) (output string, err error)

	// Down takes the stack's containers and networks away, and its volumes
	// when removeVolumes says so.
	Down(ctx context.Context, s Stack, removeVolumes bool) (output string, err error)
}

// Descriptor is the stack kind, a new one every time.
func Descriptor() kind.Descriptor {
	command := func(name string, allowedIn []kind.State, desires kind.State, permission string) kind.Action {
		return kind.Action{Name: name, Runs: kind.OnNode, Mode: kind.ModeCommand, AllowedIn: allowedIn, Desires: desires, Permission: permission, Payload: kind.NoPayload}
	}

	internal := func(name string, allowedIn ...kind.State) kind.Action {
		return kind.Action{Name: name, Runs: kind.OnNode, Mode: kind.ModeCommand, AllowedIn: allowedIn, Desires: Running, Internal: true, Payload: kind.NoPayload}
	}

	// what brings a stack up may pull its images first.
	pulling := func(a kind.Action) kind.Action {
		a.Timeout = kind.TimeoutPull

		return a
	}

	return kind.Descriptor{
		Name:    Name,
		Plural:  Plural,
		StateBy: kind.OnNode,
		Parent:  Parent,

		// a stack's containers are on its VM's disk: they go with it, and
		// are what a restored disk holds.
		OnParent: kind.ParentRules{Delete: kind.CascadeDelete, Restore: kind.CascadeReset},

		Machine: Machine(),
		Actions: []kind.Action{
			pulling(internal(ActionCreate, Waiting)),
			pulling(internal(ActionApply, Degraded, Failed)),
			pulling(command(ActionStart, []kind.State{Stopped, Degraded, Failed, Waiting}, Running, "manage")),
			command(ActionStop, []kind.State{Running, Degraded, Failed}, Stopped, "manage"),
			pulling(command(ActionRestart, []kind.State{Running, Degraded, Stopped, Failed, Waiting}, Running, "manage")),
			{Name: ActionDelete, Runs: kind.OnNode, Mode: kind.ModeCommand, Desires: Deleted, Permission: "delete", Payload: kind.Payload[DeletePayload]()},
			{Name: ActionState, Runs: kind.OnNode, Mode: kind.ModeQuery, Permission: "show", Payload: kind.NoPayload},
		},
	}
}

// Machine is a stack's states and the moves between them.
//
// A command is waited on in flight until its own answer says what it left
// the stack as: deploying, starting and restarting end in running, degraded
// or stopped, stopping in stopped, and removing only in the stack being gone.
// While a node carries a command out, it says the stack is still in flight,
// so that what the containers are doing halfway through a deploy is not taken
// for what the deploy came to.
//
// At rest, a stack is what its node says: running, degraded or stopped, or
// waiting while its VM is not running. One its VM has nothing of is waiting
// too, to be deployed again.
func Machine() kind.Machine {
	transitions := []kind.Transition{
		{From: Waiting, On: kind.OnAction(ActionCreate), To: Deploying},
		{From: Degraded, On: kind.OnAction(ActionApply), To: Deploying},
		{From: Failed, On: kind.OnAction(ActionApply), To: Deploying},
		{From: kind.Any, On: kind.OnAction(ActionDelete), To: Removing},
		{From: Removing, On: kind.OnObserved(Deleted), To: Deleted},
		{From: kind.Any, On: kind.OnObserved(Failed), To: Failed},
	}

	asked := func(action string, to kind.State, from ...kind.State) {
		for _, state := range from {
			transitions = append(transitions, kind.Transition{From: state, On: kind.OnAction(action), To: to})
		}
	}

	asked(ActionStart, Starting, Stopped, Degraded, Failed, Waiting)
	asked(ActionStop, Stopping, Running, Degraded, Failed)
	asked(ActionRestart, Restarting, Running, Degraded, Stopped, Failed, Waiting)

	arrives := func(from kind.State, at ...kind.State) {
		for _, state := range at {
			transitions = append(transitions, kind.Transition{From: from, On: kind.OnObserved(state), To: state})
		}
	}

	arrives(Deploying, Running, Degraded, Stopped)
	arrives(Starting, Running, Degraded, Stopped)
	arrives(Restarting, Running, Degraded, Stopped)
	arrives(Stopping, Stopped)

	// a stack its VM has nothing of waits to be deployed again.
	for _, state := range []kind.State{Running, Degraded, Stopped, Waiting} {
		transitions = append(transitions, kind.Transition{From: state, On: kind.OnObserved(kind.Missing), To: Waiting})
	}

	return kind.Machine{
		Initial:     Waiting,
		States:      []kind.State{Waiting, Deploying, Running, Degraded, Starting, Stopping, Stopped, Restarting, Removing, Failed, Deleted},
		Transitions: transitions,
		Terminal:    []kind.State{Stopped, Failed, Deleted},
		InFlight:    []kind.State{Deploying, Starting, Stopping, Restarting, Removing},

		// a look its node took between a command being asked and carried out
		// says the stack is as it was, which says nothing of what the command
		// came to.
		Answered: []kind.State{Deploying, Starting, Restarting},
	}
}

// InFlightOf is the state a stack is in while action is carried out on it,
// and nothing for one that leaves it where it is.
func InFlightOf(action string) kind.State {
	switch action {
	case ActionCreate, ActionApply:
		return Deploying
	case ActionStart:
		return Starting
	case ActionStop:
		return Stopping
	case ActionRestart:
		return Restarting
	case ActionDelete:
		return Removing
	}

	return ""
}
