// Package stack is the stack kind: the services, volumes and networks a
// stack's file says, made by the platform in a Docker VM, declared once for
// every service that runs it (domain/workload/kind).
//
// A stack is the root of a graph of its own (domain/workload/graph): its file
// is read by the platform (File, Parse), in a subset of compose's own format
// that the platform carries out itself, and each of its services is a
// container, each volume a volume and each network a network, records of
// their own kinds, which its graph makes in its Docker VM, in order: its VM,
// then its networks and its volumes, then its services as they depend on one
// another, and then the stack, which says of each service the container made
// of it (Status.Services). Nothing runs compose, and nothing reads what
// compose would label.
//
// Its state is the control plane's (StateBy: OnControlPlane): no node says
// what a stack is doing. While its graph makes it, it is deploying; once made,
// it is what its containers make it, running, degraded or stopped (StateOf);
// it waits while what it needs cannot be made now, its VM not running or a
// container of it being removed, stopped or not, and fails, saying why, when
// its plan does. Started, stopped and restarted, its
// graph has its containers started a level of its services at a time, as
// they depend on one another, and stopped the other way round. Deleted, it is
// removing while its graph removes its containers, and then its networks, and
// its record goes last of all; its volumes and its VM stay.
//
// A stack's spec is immutable: there is no update, only a new stack.
package stack

import (
	"context"

	"github.com/khanzadimahdi/testproject/domain/workload/kind"
	"github.com/khanzadimahdi/testproject/domain/workload/noderequest"
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
	// Deploying is a stack its graph is making, as its file says: the state
	// it is admitted in, and the one it is in again once its VM went, until
	// its graph made the VM again, and what was in it.
	Deploying kind.State = "deploying"

	// Running is a stack whose every service runs, or ran to its end as it
	// was to, and Degraded one some of whose services do not.
	Running  kind.State = "running"
	Degraded kind.State = "degraded"

	// Starting, Stopping and Restarting are a stack whose graph has its
	// containers started, stopped, or stopped and started again, a level of
	// its services at a time.
	Starting   kind.State = "starting"
	Stopping   kind.State = "stopping"
	Stopped    kind.State = "stopped"
	Restarting kind.State = "restarting"

	// Removing is a stack whose graph is removing it. Its record goes last
	// of all, once what it owns is gone.
	Removing kind.State = "removing"

	// Waiting is a stack whose graph waits on what it needs, which cannot be
	// made now: its VM is not running, or a container of it is being
	// removed. A stack deploying waits so, and so does one at rest, stopped
	// among it; one asked to start, stop or restart stays as it was asked,
	// saying what it waits on. Its reason says what it waits on.
	Waiting = kind.Waiting

	// Failed is a stack whose plan failed. Its reason, and its failure when
	// what it needs was refused, say why.
	Failed = kind.Failed

	Deleted = kind.Deleted
)

// A stack's actions, each carried out by its graph, on the control plane.
const (
	ActionStart   = "start"
	ActionStop    = "stop"
	ActionRestart = "restart"

	// ActionDelete removes a stack's containers and the networks it owns,
	// and keeps its volumes, since what is in them is usually what somebody
	// wants back: they stay in its VM until they are removed themselves.
	ActionDelete = "delete"

	// ActionState is what a stack is doing, as its record says.
	ActionState = "state"

	// ActionCreate and ActionApply had a node deploy a stack with compose,
	// which nothing asks any more: compose's path, which goes with it.
	ActionCreate = "create"
	ActionApply  = "apply"
)

// The labels the platform puts on what a stack's graph makes of its file, so
// that what is a stack's is known exactly rather than guessed from a name.
const (
	// LabelStack is the uuid of the stack a container belongs to.
	LabelStack = "workload.stack"

	// LabelServices are the services compose ran a stack's containers of,
	// comma separated, which compose's path labelled them with, and nothing
	// reads but it.
	LabelServices = "workload.stack.services"

	// LabelService is the service of a stack's file a container is, which
	// the stack's graph made of it: the record of a container a stack owns
	// says it, and the container its VM's dockerd holds is labelled with it,
	// beside the stack's uuid (LabelStack).
	LabelService = "workload.stack.service"
)

// Spec is what a stack is asked for as: the Docker VM it goes into and its
// file, neither of which ever changes.
type Spec struct {
	VM VMChoice `json:"vm"`

	// Compose is its file, as it was given (File). What compose would call
	// its project it does not say: what its graph makes is named under the
	// stack's slug.
	Compose string `json:"compose"`
}

// VMChoice is which Docker VM a stack or a container goes into.
//
// As it is asked, it names one of the person's Docker VMs by its uuid, or
// names none, which is a Docker VM made for it with whatever of the rest it
// gives and the defaults for what it leaves out: naming one and describing
// one besides is refused, as asking for two VMs (vm_or_new_vm), of what is
// asked and of nothing kept. As a stack or a container keeps it, it is the
// uuid of the VM it went into, made for it or not, and beside it what to make
// that VM with again, under that uuid, whenever it is gone: its name,
// resources, ports and network, as the VM's record had them when it was
// named, or as it was asked for with the defaults filled in when its graph is
// to make it, and as the VM's record had them when it went, and its slug with
// them, when a delete of the VM kept it.
type VMChoice struct {
	UUID string `json:"uuid,omitempty"`

	// Name, Resources, Ports and Network are what a Docker VM made for a
	// stack or a container is given beyond the defaults. Anything left out is
	// the default; ports that are there and empty are none.
	Name      string      `json:"name,omitempty"`
	Resources *Resources  `json:"resources,omitempty"`
	Ports     []port.Port `json:"ports,omitzero"`
	Network   *Network    `json:"network,omitempty"`

	// Slug is the slug the VM had, which it asks for when it is made again
	// under its uuid, so that it is reached where it was: what a stack or a
	// container keeps, and nobody asks for. What is asked never names one
	// that counts.
	Slug string `json:"slug,omitempty"`
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

// Status is what a stack is doing: its state, and each of its services, by
// the container its graph made of it.
type Status struct {
	kind.Status

	// Services are the stack's services, each with the container made of it
	// as its record said it when the stack was last made: its foreign keys
	// to its containers.
	Services []ServiceStatus `json:"services,omitempty"`

	// Output is what compose printed, when compose ran a stack, which
	// nothing says any more.
	Output string `json:"output,omitempty"`

	// Failure is what its plan failed with, when what it needs was refused,
	// in the codes every side knows: no node had room for its VM, say. One
	// that is empty, or none, is none.
	Failure *noderequest.Error `json:"failure,omitempty"`
}

// ServiceState is what one service of a stack is doing.
type ServiceState string

const (
	// ServiceRunning is a service whose container runs.
	ServiceRunning ServiceState = "running"

	// ServiceDegraded is a service some of whose containers run, of several,
	// which no service has, each being one container.
	ServiceDegraded ServiceState = "degraded"

	// ServiceStopped is a service whose container does not run, and did not
	// run to its end as it was to.
	ServiceStopped ServiceState = "stopped"

	// ServiceMissing is a service that has no container, or one not made.
	ServiceMissing ServiceState = "missing"

	// ServiceCompleted is a service that runs to its end, and has: its
	// container, which was to be running, exited with 0, and is not started
	// again, as its restart policy says. It is as it was to be.
	ServiceCompleted ServiceState = "completed"
)

// ServiceStatus is one service of a stack.
type ServiceStatus struct {
	Name       string            `json:"name"`
	State      ServiceState      `json:"state"`
	Containers []ContainerStatus `json:"containers,omitempty"`
}

// ContainerStatus is the container of a stack's service: its record's uuid,
// its Docker name, and its state, in docker's word, created, running, paused,
// restarting, removing, exited or dead, nothing before docker has it.
type ContainerStatus struct {
	ID    string `json:"id"`
	Name  string `json:"name"`
	State string `json:"state"`
}

// StateOf is what a stack is doing whose services are doing what services
// say: running when each of them runs, or ran to its end as it was to, and
// one of them runs at least; stopped when none of them runs; and degraded
// otherwise.
func StateOf(services []ServiceStatus) kind.State {
	running, fine := 0, 0

	for _, service := range services {
		switch service.State {
		case ServiceRunning:
			running++
			fine++
		case ServiceCompleted:
			fine++
		}
	}

	switch {
	case running == 0:
		return Stopped
	case fine == len(services):
		return Running
	}

	return Degraded
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

// Applier brought the containers of a stack to what its compose file said,
// inside the Docker VM it lived in, with compose: compose's path, which no
// stack takes any more, and which goes with it.
type Applier interface {
	Up(ctx context.Context, s Stack) (output string, err error)
	Start(ctx context.Context, s Stack) (output string, err error)
	Stop(ctx context.Context, s Stack) (output string, err error)
	Restart(ctx context.Context, s Stack) (output string, err error)
	Down(ctx context.Context, s Stack) (output string, err error)
}

// Descriptor is the stack kind, a new one every time.
//
// Every action of a stack is the control plane's: started, stopped,
// restarted and deleted, its record says so, and its graph carries it out.
// Nothing is asked of a node of a stack, and no node says anything of one.
func Descriptor() kind.Descriptor {
	command := func(name string, allowedIn []kind.State, desires kind.State, permission string) kind.Action {
		return kind.Action{Name: name, Runs: kind.OnControlPlane, Mode: kind.ModeCommand, AllowedIn: allowedIn, Desires: desires, Permission: permission, Payload: kind.NoPayload}
	}

	return kind.Descriptor{
		Name:    Name,
		Plural:  Plural,
		StateBy: kind.OnControlPlane,
		Parent:  Parent,

		// what a stack is, is its record, the control plane's own: deleting its
		// VM takes it with it but for its graph keeping it, as it keeps what
		// it makes, and restoring its VM's disk leaves it as it is, and resets
		// its containers, its networks and its volumes to what the disk holds.
		OnParent: kind.ParentRules{Delete: kind.CascadeDelete, Restore: kind.CascadeKeep},

		Machine: Machine(),
		Actions: []kind.Action{
			command(ActionStart, []kind.State{Stopped, Degraded, Failed, Waiting}, Running, "manage"),
			command(ActionStop, []kind.State{Running, Degraded, Failed}, Stopped, "manage"),
			command(ActionRestart, []kind.State{Running, Degraded, Stopped, Failed, Waiting}, Running, "manage"),
			{Name: ActionDelete, Runs: kind.OnControlPlane, Mode: kind.ModeCommand, Desires: Deleted, Permission: "delete", Payload: kind.NoPayload},
			{Name: ActionState, Runs: kind.OnControlPlane, Mode: kind.ModeQuery, Permission: "show", Payload: kind.NoPayload},
		},
	}
}

// Machine is a stack's states and the moves between them.
//
// A stack is admitted deploying, in flight while its graph makes it. Asked to
// start, stop or restart, it is in flight while its graph has its containers
// started or stopped, and a restart, once they are stopped, starts them. In
// flight, it is taken only where it is going: what its containers make it,
// once its graph made it or carried out what it was asked; failed, once its
// plan failed; and waiting, once its plan waits on what it needs, but only
// from deploying: asked to start, stop or restart while its graph waits, on
// its VM say, it stays as it was asked, and starts or stops once it can. At
// rest, running, degraded, stopped, failed or waiting, it is what its graph
// last said it is: what its containers make it, failed, or waiting once its
// plan waits on what it needs, its VM not running or a container of it being
// removed, a stopped stack as much as a running one. Deleted, it is removing
// until its record is gone.
func Machine() kind.Machine {
	transitions := []kind.Transition{
		{From: kind.Any, On: kind.OnAction(ActionDelete), To: Removing},
		{From: Removing, On: kind.OnObserved(Deleted), To: Deleted},
		{From: kind.Any, On: kind.OnObserved(Failed), To: Failed},
		{From: Deploying, On: kind.OnObserved(Waiting), To: Waiting},
		{From: Restarting, On: kind.OnObserved(Starting), To: Starting},
	}

	asked := func(action string, to kind.State, from ...kind.State) {
		for _, state := range from {
			transitions = append(transitions, kind.Transition{From: state, On: kind.OnAction(action), To: to})
		}
	}

	asked(ActionStart, Starting, Stopped, Degraded, Failed, Waiting)
	asked(ActionStop, Stopping, Running, Degraded, Failed)
	asked(ActionRestart, Restarting, Running, Degraded, Stopped, Failed, Waiting)

	// what its containers make it, once its graph made it, or carried out
	// what it was asked.
	for _, from := range []kind.State{Deploying, Starting, Stopping} {
		for _, at := range []kind.State{Running, Degraded, Stopped} {
			transitions = append(transitions, kind.Transition{From: from, On: kind.OnObserved(at), To: at})
		}
	}

	return kind.Machine{
		Initial:     Deploying,
		States:      []kind.State{Deploying, Running, Degraded, Starting, Stopping, Stopped, Restarting, Removing, Waiting, Failed, Deleted},
		Transitions: transitions,
		Terminal:    []kind.State{Stopped, Failed, Deleted},
		InFlight:    []kind.State{Deploying, Starting, Stopping, Restarting, Removing},
	}
}

// InFlightOf is the state a node said a stack was in while compose carried
// action out on it, which nothing asks of a node any more: compose's path,
// which goes with it.
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
