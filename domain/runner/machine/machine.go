// Package machine is what an orchestrator asks of the host its microVMs run
// on, and what the host does to answer.
//
// Starting a microVM is two jobs. One is deciding what it is — its disks, its
// kernel, its task — which is the orchestrator's, and needs no privilege at
// all. The other is giving it a process of its own with a way to the hardware,
// and plugging it into the host's network, which only something privileged on
// the host can do. The launcher does the second and nothing else, so the part
// that is reachable from outside holds no privilege, and the part that holds
// it is reachable only through a socket on the host.
package machine

import (
	"context"
	"errors"
	"regexp"
)

// Spec is a machine to launch.
type Spec struct {
	// ID names the machine. It is sixteen hex digits, which keeps every path
	// and device named after it short enough for the kernel to take.
	ID string `json:"id"`

	// Owner is the orchestrator the machine belongs to. Several share a host,
	// and each only ever sees its own.
	Owner string `json:"owner"`

	// VCPUs is how many CPUs the machine has, and CPUQuota how much of them
	// it may use: half a CPU is one CPU used half the time.
	VCPUs    int     `json:"vcpus"`
	CPUQuota float64 `json:"cpu_quota,omitempty"`

	// MemoryMiB is the machine's memory.
	MemoryMiB int `json:"memory_mib"`

	// Taps are the networks the machine is plugged into, in the order its
	// network devices are to find them.
	Taps []Tap `json:"taps,omitempty"`

	// Files are what the machine boots from, as they are on the host.
	Files Files `json:"files"`
}

// Tap is one of a machine's network devices, and the network its host end is
// plugged into.
type Tap struct {
	Network string `json:"network"`
}

// Files are what a machine boots from: a kernel, the initramfs holding its
// agent, and its disks in the order the machine finds them.
type Files struct {
	Kernel string   `json:"kernel"`
	Initrd string   `json:"initrd"`
	Drives []string `json:"drives,omitempty"`
}

// Machine is a machine the host holds.
type Machine struct {
	ID    string `json:"id"`
	Owner string `json:"owner"`

	// PID is the process the machine runs in, and Running whether it still
	// does. A machine that is no longer running is still held until it is
	// terminated, so its end can be told apart from its never having been.
	PID     int  `json:"pid"`
	Running bool `json:"running"`

	// Socket is where the machine's firecracker takes its API on the host.
	Socket string `json:"socket"`

	// Root is the machine's own directory on the host. A path given to its
	// firecracker is relative to it, and whatever firecracker makes there —
	// its vsock's socket — is found under it.
	Root string `json:"root"`

	// Files are what it boots from, as its firecracker is to be told them.
	Files Files `json:"files"`

	// Taps are its network devices' host ends, as its firecracker is to be
	// told them.
	Taps []AttachedTap `json:"taps,omitempty"`
}

// AttachedTap is a tap device on the host, plugged into a network.
type AttachedTap struct {
	Network string `json:"network"`
	Device  string `json:"device"`
}

// Network is one of the host's networks.
type Network struct {
	Owner string `json:"owner"`
	Name  string `json:"name"`

	// Subnet is the network's own, and Gateway the host's address on it.
	Subnet  string `json:"subnet"`
	Gateway string `json:"gateway"`

	// Masquerade is whether it routes out to the internet.
	Masquerade bool `json:"masquerade"`
}

// Launcher is what an orchestrator asks of the host its machines run on.
type Launcher interface {
	// Launch starts a machine's process and plugs it into its networks. The
	// machine waits to be configured through its socket, and runs nothing
	// until it is.
	Launch(ctx context.Context, spec Spec) (Machine, error)

	// Terminate ends a machine's process, unplugs it, and lets go of
	// everything the host kept for it. A machine that is not there is the
	// outcome asked for.
	Terminate(ctx context.Context, id string) error

	// Machines is every machine the host holds for owner, running or not.
	Machines(ctx context.Context, owner string) ([]Machine, error)

	// EnsureNetwork makes one of owner's networks, if it is not there
	// already, and says what it is.
	EnsureNetwork(ctx context.Context, owner string, name string, masquerade bool) (Network, error)

	// RemoveNetwork takes one of owner's networks away. It refuses while
	// anything is still plugged into it; a network that is not there is the
	// outcome asked for.
	RemoveNetwork(ctx context.Context, owner string, name string) error
}

// VMM starts, ends and finds the processes machines run in.
type VMM interface {
	Spawn(ctx context.Context, spec Spec, taps []AttachedTap) (Machine, error)
	Kill(ctx context.Context, id string) error
	List(ctx context.Context) ([]Machine, error)
}

// HostNetwork is the host's side of machines' networks.
type HostNetwork interface {
	EnsureNetwork(ctx context.Context, owner string, name string, masquerade bool) (Network, error)
	RemoveNetwork(ctx context.Context, owner string, name string) error

	// Plug makes a machine's tap devices and plugs each into its network;
	// Unplug takes them away again.
	Plug(ctx context.Context, owner string, id string, taps []Tap) ([]AttachedTap, error)
	Unplug(ctx context.Context, id string) error
}

var (
	idPattern      = regexp.MustCompile(`^[0-9a-f]{16}$`)
	namePattern    = regexp.MustCompile(`^[a-z0-9][a-z0-9-]{0,62}$`)
	networkPattern = regexp.MustCompile(`^runner-[a-z0-9-]{1,56}$`)
)

// IsID reports whether id can name a machine.
func IsID(id string) bool {
	return idPattern.MatchString(id)
}

// IsOwner reports whether owner can name an orchestrator.
func IsOwner(owner string) bool {
	return namePattern.MatchString(owner)
}

// IsNetwork reports whether name can name one of the runner's networks.
func IsNetwork(name string) bool {
	return networkPattern.MatchString(name)
}

// ErrNetworkInUse is a network that still has machines plugged into it.
var ErrNetworkInUse = errors.New("the network still has machines plugged into it")
