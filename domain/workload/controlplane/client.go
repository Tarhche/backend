package controlplane

import (
	"context"
	"time"

	"github.com/khanzadimahdi/testproject/domain/workload/docker"
	taskKind "github.com/khanzadimahdi/testproject/domain/workload/kinds/task"
	"github.com/khanzadimahdi/testproject/domain/workload/port"
	"github.com/khanzadimahdi/testproject/domain/workload/snapshot"
	"github.com/khanzadimahdi/testproject/domain/workload/stack"
	"github.com/khanzadimahdi/testproject/domain/workload/vm"
)

// Page is one page of a listing.
type Page[T any] struct {
	Items       []T
	TotalPages  uint
	CurrentPage uint
}

// Client is the workload, as the blog reaches it.
//
// An ownerUUID narrows what a method acts on to that person's own: something
// that is not theirs is not there as far as they are concerned, and is
// reported as domain.ErrNotExists. Empty is anybody's, which is what the
// dashboard's admin routes ask for. A method that creates something creates it
// for ownerUUID, whoever's routes asked.
//
// What the workload refuses comes back as an error the caller can tell apart:
// domain.ErrNotExists for something that is not there, a validation error for
// a request it would not take, and a *noderequest.Error for what a node
// refused, which errors.Is matches against the domain's own errors.
type Client interface {
	// RunTask asks for a task to be run, for ownerUUID, admitted as the
	// control plane admits any resource: checked, given its defaults and a
	// slug, placed on a node and asked of it. The code runner runs every
	// snippet as a task of the guest's.
	RunTask(ctx context.Context, ownerUUID string, request TaskRequest) (taskKind.Task, error)

	// Task is one task the workload holds, whoever owns it. The code runner
	// reads one back to make sure it is its own before it takes it away.
	Task(ctx context.Context, uuid string) (taskKind.Task, error)

	// DeleteTask removes a task whether or not it is still running: a delete
	// is a request to have it gone.
	DeleteTask(ctx context.Context, uuid string) error

	// VMs is a page of VMs.
	VMs(ctx context.Context, ownerUUID string, page uint) (Page[vm.VM], error)
	VM(ctx context.Context, ownerUUID string, uuid string) (vm.VM, error)
	CreateVM(ctx context.Context, ownerUUID string, request VMRequest) (vm.VM, error)
	UpdateVM(ctx context.Context, ownerUUID string, uuid string, update VMUpdate) (vm.VM, error)

	// DeleteVM asks for a VM to be removed. Its snapshots stay.
	DeleteVM(ctx context.Context, ownerUUID string, uuid string) error

	StartVM(ctx context.Context, ownerUUID string, uuid string) error
	StopVM(ctx context.Context, ownerUUID string, uuid string) error
	RestartVM(ctx context.Context, ownerUUID string, uuid string) error

	// RestoreVM replaces a VM's disk from a snapshot of the same owner, kind
	// and engine. The VM keeps its uuid, its slug and its ports.
	RestoreVM(ctx context.Context, ownerUUID string, uuid string, snapshotUUID string) error

	// VMLogs is the tail of a VM's log, read from its node as it is now.
	VMLogs(ctx context.Context, ownerUUID string, uuid string, options vm.LogOptions) ([]vm.LogLine, error)

	// Snapshots is a page of snapshots, narrowed to one VM's unless vmUUID is
	// empty.
	Snapshots(ctx context.Context, ownerUUID string, vmUUID string, page uint) (Page[snapshot.Snapshot], error)
	Snapshot(ctx context.Context, ownerUUID string, uuid string) (snapshot.Snapshot, error)

	// CreateSnapshot takes a snapshot of a VM's disk, for ownerUUID.
	CreateSnapshot(ctx context.Context, ownerUUID string, vmUUID string, name string) (snapshot.Snapshot, error)
	RenameSnapshot(ctx context.Context, ownerUUID string, uuid string, name string) (snapshot.Snapshot, error)

	// DeleteSnapshot removes a snapshot and its archive.
	DeleteSnapshot(ctx context.Context, ownerUUID string, uuid string) error

	// Docker is the building blocks of a Docker VM, its containers, images,
	// networks and volumes, as the control plane keeps them: what was made
	// through it is a resource of its kind, stored and brought back to what
	// was asked of it, and is shown as its node last reported it; what the
	// VM's dockerd holds besides, a stack's or what was made from the VM's
	// terminal, is shown beside it as its node reported it, and marked
	// unmanaged when it is nobody's. Whatever is made is waited for, for as
	// long as dockerd may take, and a log or a sample of what a container
	// uses is read from dockerd as it is now. A VM that is not a Docker VM is
	// refused by every call, and so is one that is neither running nor on its
	// way up.
	//
	// It is the domain's own Daemon rather than a method per operation here,
	// so that what is asked of a Docker VM through the control plane and what
	// a node asks of it directly are one interface.
	Docker(ownerUUID string, vmUUID string) docker.Daemon

	// CreateContainer creates a container in the Docker VM the request
	// chooses, making that VM first when it says so or when ownerUUID has
	// none.
	CreateContainer(ctx context.Context, ownerUUID string, request ContainerRequest) (CreatedContainer, error)

	// Containers is every container across the running Docker VMs, each with
	// the VM it is in, narrowed to one VM's unless vmUUID is empty.
	Containers(ctx context.Context, ownerUUID string, vmUUID string) ([]VMContainer, error)

	// Stacks is a page of stacks, narrowed to one VM's unless vmUUID is
	// empty.
	Stacks(ctx context.Context, ownerUUID string, vmUUID string, page uint) (Page[stack.Stack], error)

	// Stack is one stack and the containers compose made for it.
	Stack(ctx context.Context, ownerUUID string, uuid string) (StackDetail, error)

	// CreateStack deploys a compose project into the Docker VM the request
	// chooses, making that VM first when it says so or when ownerUUID has
	// none.
	CreateStack(ctx context.Context, ownerUUID string, request StackRequest) (CreatedStack, error)

	// DeleteStack takes a stack down and removes it, and its volumes with it
	// when removeVolumes says so.
	DeleteStack(ctx context.Context, ownerUUID string, uuid string, removeVolumes bool) error

	StartStack(ctx context.Context, ownerUUID string, uuid string) error
	StopStack(ctx context.Context, ownerUUID string, uuid string) error
	RestartStack(ctx context.Context, ownerUUID string, uuid string) error
}

// TaskRequest is a task to run: what it is called, and what it runs.
type TaskRequest struct {
	Name string
	Spec taskKind.Spec
}

// VMRequest is a VM to create.
type VMRequest struct {
	Name string
	Kind vm.Kind

	// Image is what it boots from, which is what makes it a Docker VM or a
	// machine; empty is the Docker image for a Docker VM, and the default one
	// for a machine.
	Image string

	Resources      vm.Resources
	Ports          []port.Port
	Network        vm.Network
	PersistentDisk bool

	// Lifetime is how long it is kept; zero keeps it until it is deleted.
	Lifetime time.Duration

	// SnapshotUUID, when set, makes the VM from that snapshot: its kind and
	// image are the snapshot's, and its disk is the larger of the one asked
	// for and the snapshot's.
	SnapshotUUID string
}

// VMUpdate is what changes about a VM. Anything left nil stays as it is; its
// kind and its image never change, and its disk only grows. A change to its
// ports, network or resources restarts a VM that is not stopped.
type VMUpdate struct {
	Name *string

	// Lifetime counts again from now; zero keeps the VM until it is deleted.
	Lifetime *time.Duration

	Ports     *[]port.Port
	Network   *vm.Network
	Resources *vm.Resources
}

// DockerVMChoice is which Docker VM a container or a stack goes into.
//
// One that names a VM goes into it, and it has to be a Docker VM of the
// person asking. One that names none goes into their only Docker VM, or into
// one made for it with the defaults when they have none; with several to
// choose from, it is refused as vm_required.
type DockerVMChoice struct {
	UUID string

	// New makes a Docker VM for it with these settings, the defaults filling
	// in what they leave out.
	New *NewDockerVM
}

// NewDockerVM is what a Docker VM made for a container or a stack is given
// beyond the defaults. Anything left empty is the default.
type NewDockerVM struct {
	Name      string
	Resources *vm.Resources
	Ports     []port.Port
	Network   *vm.Network
}

// ChosenVM is the Docker VM a container or a stack went into, and whether it
// was made for it.
type ChosenVM struct {
	UUID    string
	Name    string
	Created bool
}

// ContainerRequest is a container to create, and where.
type ContainerRequest struct {
	VM        DockerVMChoice
	Container docker.ContainerSpec
}

// CreatedContainer is a container that was created, and the VM it is in.
type CreatedContainer struct {
	VM        ChosenVM
	Container docker.Container
}

// VMContainer is a container together with the Docker VM it runs in.
type VMContainer struct {
	docker.Container

	VMUUID string
	VMName string
}

// StackRequest is a compose project to deploy, and where.
type StackRequest struct {
	Name string

	// Compose is the YAML, as it was written.
	Compose string

	VM DockerVMChoice
}

// CreatedStack is a stack that was deployed, and the VM it is in.
type CreatedStack struct {
	VM    ChosenVM
	Stack stack.Stack
}

// StackDetail is a stack and the containers compose made for it, as its
// node last reported them.
type StackDetail struct {
	stack.Stack

	Containers []docker.Container

	// VMNotRunning says there are no containers to show because the stack's
	// VM is not running, rather than because the stack has none.
	VMNotRunning bool
}
