// Package vm is a user's virtual machine, as the workload keeps it.
//
// A VM is a record the control plane owns: what it was asked to be, what its
// node last said it is, and where it runs. What actually runs it is an Engine
// on one node, which knows nothing about records and answers only for the
// instances it holds.
package vm

import (
	"context"
	"time"

	"github.com/khanzadimahdi/testproject/domain/workload/port"
)

// Kind is what a VM boots into.
type Kind string

const (
	// KindMachine boots an operating system image with no main process: it is
	// a machine somebody opens a terminal in, and it runs until it is stopped.
	KindMachine Kind = "machine"

	// KindDocker boots a docker-in-docker image with dockerd running, which is
	// what containers and stacks are run in. A VM is a Docker VM because it was
	// made as one; nothing ever looks inside a VM to find a docker daemon.
	KindDocker Kind = "docker"
)

// IsValid reports whether k is one of the known kinds.
func (k Kind) IsValid() bool {
	switch k {
	case KindMachine, KindDocker:
		return true
	default:
		return false
	}
}

func (k Kind) String() string {
	return string(k)
}

// Access is whether one direction of a VM's network is open.
type Access string

const (
	AccessAllow Access = "allow"
	AccessDeny  Access = "deny"
)

// IsValid reports whether a is one of the known accesses.
func (a Access) IsValid() bool {
	switch a {
	case AccessAllow, AccessDeny:
		return true
	default:
		return false
	}
}

func (a Access) String() string {
	return string(a)
}

// Network is how much of the network a VM has. Nothing ever lets one VM reach
// another, whatever either of them allows.
type Network struct {
	// Ingress allowed makes the VM's Ports reachable through the ingress;
	// denied, nothing reaches it at all.
	Ingress Access

	// Egress allowed lets the VM reach the public internet, never private
	// ranges, the host or other VMs; denied, it reaches nothing.
	Egress Access
}

// Resources are what a VM is given. CPUs are whole vCPUs, and Memory and Disk
// are bytes, as they are everywhere in the workload: nothing between where
// they are asked for and the engine converts them.
type Resources struct {
	CPUs   uint
	Memory uint64
	Disk   uint64
}

// VM is one virtual machine.
type VM struct {
	UUID string
	Name string

	// Slug is the name its ports are served under, unique across VMs and
	// tasks: the left-most label of the hostname the ingress answers for it.
	Slug string

	OwnerUUID string
	Kind      Kind

	// Image is the OCI reference it boots from. A VM that names none takes its
	// kind's default, and it never changes once the VM exists.
	Image string

	Resources Resources

	// Ports are the guest ports exposed through the ingress, sorted and with
	// none twice.
	Ports []port.Port

	Network Network

	// PersistentDisk keeps what is written to the disk across a stop and a
	// start. Without it the disk is pristine on every start, as the image left
	// it.
	PersistentDisk bool

	// Lifetime is how long the VM is kept. Zero keeps it until it is deleted;
	// anything else deletes it at ExpiresAt.
	Lifetime  time.Duration
	ExpiresAt time.Time

	// CurrentState is what the VM is doing, as its node last reported.
	// ExpectedState is what it was asked to be doing. Closing the gap between
	// the two is what the control plane's own heartbeat does.
	CurrentState  State
	ExpectedState State

	// Reason is why it failed, or what is pending, when the workload can say.
	Reason string

	// NodeName is the node holding it. A VM lives on one node for its whole
	// life, because its disk is there.
	NodeName string

	// Stats is the last sample its node reported.
	Stats Stats

	// RestoreFrom is the snapshot a pending restore is from. It is cleared once
	// the VM has been restored.
	RestoreFrom string

	// LastHeartbeatAt is when its node last said anything about it. A VM
	// nobody has spoken for in a while is one that is no longer there,
	// whatever it was last seen doing.
	LastHeartbeatAt time.Time

	CreatedAt time.Time
	StartedAt time.Time
	UpdatedAt time.Time

	// ManagedBy names what keeps a VM that is not a record of its own, such as
	// ManagedByCodeRunner. It is empty for every VM somebody asked for, which
	// is every VM that is stored: it is only ever read, never kept.
	ManagedBy string
}

// ManagedByCodeRunner is what keeps a VM the code runner runs a snippet in.
//
// Each snippet runs in a VM of its own for as long as it runs, as a task of
// the guest's, and the task is the one record of it: the VM is that task, read
// as one. It can be stopped, deleted and read, and nothing else: it is gone
// once the snippet has ended anyway.
const ManagedByCodeRunner = "code-runner"

// Expired reports whether a VM has outlived the lifetime it was given. One
// kept until it is deleted never expires.
func (v *VM) Expired(now time.Time) bool {
	if v.Lifetime <= 0 || v.ExpiresAt.IsZero() {
		return false
	}

	return !now.Before(v.ExpiresAt)
}

// Silent reports whether nobody has spoken for this VM in a while.
//
// A VM is spoken for by the node holding it, in every heartbeat; one that has
// gone quiet was removed behind the workload's back, or its node is gone, and
// either way what it was last seen doing is no longer what it is doing.
func (v *VM) Silent(now time.Time, after time.Duration) bool {
	last := v.LastHeartbeatAt

	// one nobody has ever spoken for has been quiet since it was asked for,
	// which is what a VM whose node never took it looks like.
	if last.IsZero() {
		last = v.CreatedAt
	}

	if last.IsZero() {
		return false
	}

	return now.Sub(last) > after
}

// Drifted reports whether this VM is not doing what it was asked to do.
//
// A VM on its way somewhere — starting, stopping, restarting, being restored —
// has not drifted: it is on its way. Neither has one whose expectation was
// never set.
func (v *VM) Drifted(now time.Time, silentAfter time.Duration) bool {
	// asking for a VM to be stopped is asking for it not to be running, which
	// one that failed already is not.
	if v.ExpectedState == Stopped && IsTerminalState(v.CurrentState) {
		return false
	}

	if v.ExpectedState == 0 || v.ExpectedState == v.CurrentState {
		// unless it has gone quiet while it was supposed to be running, in
		// which case what it is doing is nothing.
		return v.ExpectedState == Running && v.Silent(now, silentAfter)
	}

	if IsInFlightState(v.CurrentState) && !v.Silent(now, silentAfter) {
		return false
	}

	return true
}

// Stats is one sample of what a VM is using.
type Stats struct {
	// CPUPercent is how busy the VM kept the vCPUs it was given, as a share of
	// all of them together: 0 is idle and 100 is every one of them busy,
	// however many it has. It is never counted per vCPU, so a VM with two
	// vCPUs both busy is 100, not 200; an engine that counts per vCPU divides
	// by the VM's vCPUs before it says so. It means the same from the engine
	// to the dashboard, and nothing on the way converts it.
	CPUPercent float64

	// Memory and disk are bytes; the network counters are bytes received and
	// sent since the VM started.
	MemoryUsed  uint64
	MemoryLimit uint64
	DiskUsed    uint64
	DiskTotal   uint64
	NetworkRx   uint64
	NetworkTx   uint64

	SampledAt time.Time
}

// Repository stores VMs.
type Repository interface {
	GetAll(ctx context.Context, offset uint, limit uint) ([]VM, error)

	// GetAllByOwner is the same listing, of one person's own.
	GetAllByOwner(ctx context.Context, ownerUUID string, offset uint, limit uint) ([]VM, error)

	// GetAllByOwnerAndKind is every VM of one kind somebody has: the Docker VMs
	// a container or a stack can be put in.
	GetAllByOwnerAndKind(ctx context.Context, ownerUUID string, kind Kind) ([]VM, error)

	// GetAllByNode is every VM one node holds, which is what its heartbeats are
	// read against.
	GetAllByNode(ctx context.Context, nodeName string) ([]VM, error)

	CountByOwner(ctx context.Context, ownerUUID string) (uint, error)
	Count(ctx context.Context) (uint, error)

	GetOne(ctx context.Context, uuid string) (VM, error)

	// GetOneByOwner is one of somebody's own. A VM that is not theirs is not
	// there as far as they are concerned.
	GetOneByOwner(ctx context.Context, ownerUUID string, uuid string) (VM, error)

	// GetOneBySlug finds a VM by the name its ports are served under.
	GetOneBySlug(ctx context.Context, slug string) (VM, error)

	Save(ctx context.Context, v *VM) (uuid string, err error)
	Delete(ctx context.Context, uuid string) error
}
