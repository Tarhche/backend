package vm

import (
	"context"
	"net"
	"regexp"
	"time"

	"github.com/khanzadimahdi/testproject/domain/workload/guest"
)

// Hypervisor boots machines from a description that says nothing about which
// VMM does the booting, so that another one — Cloud Hypervisor, QEMU's
// microvm — can be put in its place. Kernel, initramfs, virtio block, network
// and vsock devices are common to all of them.
//
// A machine is a VMM process of its own, running as a host user of its own.
// Where it runs — a host systemd unit that outlives vmhost, or a child of
// vmhost for development — is the hypervisor's to decide, and its to find
// again: vmhost keeps no process handles, only IDs.
type Hypervisor interface {
	// Name is the VMM, such as "firecracker", and Version its version, as
	// vmhost reports them.
	Name() string
	Version() string

	// Boot starts a machine's VMM and boots it. Once it returns, the guest's
	// kernel is starting, and its agent answers at the machine's VsockPath
	// moments later: waiting for it is the guest client's.
	Boot(ctx context.Context, spec MachineSpec) (Machine, error)

	// Machine is one machine the hypervisor holds, running or left behind,
	// or ErrNotFound. It is how vmhost finds again the machines it booted
	// before it restarted.
	Machine(ctx context.Context, id string) (Machine, error)

	// Machines is every machine the hypervisor holds, including ones nothing
	// in vmhost's records accounts for any more.
	Machines(ctx context.Context) ([]Machine, error)

	// Terminate ends a machine's VMM, if it still runs, and lets go of
	// everything the hypervisor kept for it. A machine that is not there is
	// the outcome asked for. The guest is asked to turn itself off first,
	// through its agent, by whoever terminates it; this does not ask.
	Terminate(ctx context.Context, id string) error
}

// MachineSpec is a machine to boot: nothing in it is Firecracker's alone.
type MachineSpec struct {
	// ID is the VM's.
	ID string

	// VCPUs is how many CPUs the machine has. CPU is the share of them it
	// may keep busy, enforced by a quota where the hypervisor can set one;
	// zero is all of them.
	VCPUs int
	CPU   float64

	MemoryMiB int

	// Kernel and Initrd are what the machine boots, and KernelArgs its
	// command line (guest.KernelArgs).
	Kernel     string
	Initrd     string
	KernelArgs string

	// Drives are the machine's disks, in the order it finds them: the image,
	// then the scratch disk (guest.ImageDevice, guest.ScratchDevice).
	Drives []Drive

	// NICs are its network devices, in the order it finds them. None is no
	// network device at all.
	NICs []NIC

	// UID is the host user the VMM runs as, with a group of the same number.
	// Zero runs it as vmhost itself, which is for development and nothing
	// else.
	UID int
}

// Drive is one of a machine's disks.
//
// Every path a machine boots from is on vmhost's data directory, on one
// filesystem: it is linked into the machine's own directory rather than
// copied. One that is read only may be shared by many machines, as an image
// is, and is everybody's to read; one that is not is the machine's own, and is
// made its user's alone while it runs.
type Drive struct {
	Path     string
	ReadOnly bool
}

// NIC is one of a machine's network devices: the tap on the host's side, and
// the MAC the guest finds it by.
type NIC struct {
	Device string
	MAC    string
}

// Machine is a machine as the hypervisor holds it.
type Machine struct {
	ID string

	// Running says the machine's VMM still runs. A machine whose VMM has
	// ended is still held until it is terminated, so that its end can be
	// told apart from its never having been.
	Running bool

	// PID is the VMM's process on the host, and UID who it runs as.
	PID int
	UID int

	// VsockPath is the unix socket the guest's agent is reached through on
	// the host (guest.Port).
	VsockPath string

	// Dir is the machine's own directory, where its VMM runs.
	Dir string

	// Cgroup is the machine's own cgroup, whose files say what it uses.
	// Empty is a machine with none of its own.
	Cgroup string

	// Unit is the host systemd unit running it, when one does.
	Unit string
}

// ImageStore makes the OCI images tasks name into disks a machine boots.
type ImageStore interface {
	// Ensure makes sure an image is here, pulling it for the host's platform
	// and making it into a disk if it is not, and says what it is. An image
	// already made under the reference it was asked by is taken as it is,
	// without asking its registry again, as a container runtime does not
	// pull an image it holds.
	Ensure(ctx context.Context, reference string) (Image, error)

	// List is every image made here.
	List(ctx context.Context) ([]Image, error)

	// Remove lets go of an image's disk. Whether a VM still boots it is
	// vmhost's to know, and to check first.
	Remove(ctx context.Context, digest string) error

	// MakeScratch makes the disk a writable task keeps its changes on, at
	// path: empty, sparse, formatted, and as large as size bytes, which is
	// the most the task can write.
	MakeScratch(ctx context.Context, path string, size uint64) error
}

// Image is an image made into a disk a machine can boot.
type Image struct {
	// Reference is what it was last asked for by, and Digest what it is.
	Reference string `json:"reference,omitempty"`
	Digest    string `json:"digest"`

	// Root is the read-only disk holding the image's filesystem, shared by
	// every VM that runs it, and Size how large it is in bytes.
	Root string `json:"root"`
	Size int64  `json:"size"`

	Config ImageConfig `json:"config"`

	// LastUsedAt is when a VM was last made from it, for letting go of the
	// ones nothing uses when the cache is full.
	LastUsedAt time.Time `json:"last_used_at,omitzero"`
}

// ImageConfig is what an image says about how it is run.
type ImageConfig struct {
	Entrypoint   []string `json:"entrypoint,omitempty"`
	Cmd          []string `json:"cmd,omitempty"`
	Env          []string `json:"env,omitempty"`
	WorkingDir   string   `json:"working_dir,omitempty"`
	User         string   `json:"user,omitempty"`
	ExposedPorts []uint16 `json:"exposed_ports,omitempty"`
}

// PublicNetwork is the network a VM that reaches the internet joins besides
// its own. It is masqueraded out, to nothing private or local; VMs on it do
// not reach each other there, since all it gives them is the way out. vmhost
// makes it when it starts.
const PublicNetwork = "public"

// networkPattern is what a network can be called. It is what the orchestrator
// calls the network: workload-isolated, workload-stack-<slug>.
var networkPattern = regexp.MustCompile(`^[a-z0-9][a-z0-9-]{0,62}$`)

// IsNetworkName reports whether name can name a network.
func IsNetworkName(name string) bool {
	return networkPattern.MatchString(name)
}

// Fabric gives machines their networks: a bridge each, a tap per device, an
// address per tap, and the firewall that says what may reach what. Machines
// on one network reach each other there; a public network also reaches the
// internet, and nothing private or local; nothing on any of them reaches the
// host, vmhost, or the platform's own services.
//
// It all lives in one network namespace of its own that outlives vmhost, so
// that a vmhost that is redeployed finds its networks where it left them, and
// docker's firewall on the host never sees a VM's traffic.
type Fabric interface {
	// EnsureNetwork makes a network, if it is not there already, and says
	// what it is. Whether a network routes out is set when it is made; asking
	// for one that is there with the other answer is refused.
	EnsureNetwork(ctx context.Context, name string, masquerade bool) (Network, error)

	// RemoveNetwork takes a network away. It is refused with
	// ErrNetworkInUse while anything is plugged into it; one that is not
	// there is the outcome asked for.
	RemoveNetwork(ctx context.Context, name string) error

	// Networks is every network there is.
	Networks(ctx context.Context) ([]Network, error)

	// Plug gives a machine a tap on each network it joins, owned by the user
	// its VMM runs as so that it needs no privilege to open it, and an
	// address on each. It says what the machine's devices are, in the order
	// the attachments were given.
	Plug(ctx context.Context, id string, uid int, attachments []Attachment) ([]Interface, error)

	// Unplug takes a machine's taps away, and gives back its addresses.
	Unplug(ctx context.Context, id string) error

	// Retain gives back the taps and addresses of every machine not named,
	// which is what is left of machines that went while nobody was looking.
	Retain(ctx context.Context, ids []string) error

	// Repair puts the firewall back as the networks say it should be, which
	// something else on the host may have changed.
	Repair(ctx context.Context) error
}

// Network is one of vmhost's networks.
type Network struct {
	Name string `json:"name"`

	// Subnet is the network's own, and Gateway the fabric's address on it,
	// which its machines route through.
	Subnet  string `json:"subnet"`
	Gateway string `json:"gateway"`

	// Masquerade says the network routes out to the internet.
	Masquerade bool `json:"masquerade"`
}

// GuestClient speaks to the agent inside one machine (domain/workload/guest),
// through the unix socket its VMM exposes for its vsock.
type GuestClient interface {
	// Ready waits until the agent answers, which is when a machine has
	// booted, and refuses an agent whose protocol version it does not know.
	Ready(ctx context.Context) error

	// Configure tells a machine what it is, once, before its task runs.
	Configure(ctx context.Context, config guest.Config) error

	// SetHosts replaces the names the machine's neighbours answer to.
	SetHosts(ctx context.Context, hosts []guest.Host) error

	// Start runs the machine's task, again if it has run before.
	Start(ctx context.Context, process guest.Process) (guest.Status, error)

	// Status is what has become of the machine's task, and Wait waits for
	// the given run of it to end. A run that has ended is answered at once.
	Status(ctx context.Context) (guest.Status, error)
	Wait(ctx context.Context, generation uint64) (guest.Status, error)

	// Signal sends a signal to the task's own process. Stop sends it TERM,
	// gives it timeout to go, and then ends it and everything it started.
	Signal(ctx context.Context, signal int) error
	Stop(ctx context.Context, timeout time.Duration) (guest.Status, error)

	// Stats is what the task uses, as the guest sees it.
	Stats(ctx context.Context) (guest.Stats, error)

	// PowerOff turns the machine off, which ends its VMM.
	PowerOff(ctx context.Context) error

	// Logs hands every line the task wrote after the one numbered after to
	// emit, in order; with follow it keeps waiting for more until ctx is
	// done, emit refuses a line, or the agent goes away.
	Logs(ctx context.Context, after uint64, follow bool, emit func(guest.LogLine) error) error

	// Exec starts a command inside the machine and hands back what the agent
	// calls it, and the connection its input and output travel on as frames
	// (guest.ReadFrame).
	Exec(ctx context.Context, exec guest.Exec) (id string, conn net.Conn, err error)

	// EndExec ends a command and everything it started, once nobody is
	// attached to it any more.
	EndExec(ctx context.Context, id string, end guest.EndExec) (guest.Ended, error)

	// Dial connects to one of the task's ports, as its neighbours on its own
	// network would. The connection carries raw bytes.
	Dial(ctx context.Context, port uint16) (net.Conn, error)

	// Close lets go of the connections kept for the next request.
	Close() error
}

// GuestConnector makes the client for one machine's agent.
type GuestConnector interface {
	Connect(vsockPath string) GuestClient
}

// StateStore keeps what vmhost knows about its VMs, so that a vmhost that
// restarts knows what it was running. It is read far more than it is written —
// a node's heartbeat lists its VMs several times a second — so it answers from
// memory, and writes every change through before it says it is made.
type StateStore interface {
	// All is every VM, oldest first.
	All(ctx context.Context) ([]VM, error)

	// Get is one VM, or ErrNotFound.
	Get(ctx context.Context, id string) (VM, error)

	// Put keeps a VM, written whole, so that it is never read half written.
	Put(ctx context.Context, vm VM) error

	// Update changes one VM under the store's lock, so that two changes to
	// it never undo each other, and says what it became. ErrNotFound when
	// there is no such VM.
	Update(ctx context.Context, id string, change func(*VM)) (VM, error)

	// Remove forgets a VM, and everything kept beside its record: its scratch
	// disk and its logs.
	Remove(ctx context.Context, id string) error
}
