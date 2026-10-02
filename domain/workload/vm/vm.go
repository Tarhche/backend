// Package vm is vmhost's model: the microVMs it runs, what it makes them
// from, and the parts it makes them with.
//
// vmhost is to Firecracker what dockerd is to containers. It is the one part
// of the workload that holds privilege on the host, and it is reached only
// through a unix socket, by the orchestrator on the same host. The
// orchestrator stays unprivileged: its microvm driver asks vmhost for VMs the
// way the container driver asks docker for containers.
//
// A VM outlives both: its VMM runs as a host systemd unit rather than as a
// child of anything that is redeployed, and its network lives in a namespace
// that outlives vmhost's container, so an orchestrator or vmhost that is
// redeployed finds its VMs where it left them and takes them back.
//
// The parts vmhost is made of are the interfaces here, so that each can be
// replaced on its own: a Hypervisor boots machines (Firecracker first), an
// ImageStore makes OCI images into disks, a Fabric gives machines their
// networks, a GuestClient speaks to the agent inside a machine, and a
// StateStore keeps what vmhost knows across its restarts. What vmhost says to
// the orchestrator is api.go.
package vm

import (
	"regexp"
	"slices"
	"strings"
	"time"

	"github.com/khanzadimahdi/testproject/domain/workload/guest"
)

// State is where a VM is in its life. The states are docker's, so that the
// orchestrator's microvm driver maps them onto task.Status one to one.
type State string

const (
	// StateCreated is a VM whose disks and record are made, and which has not
	// been booted.
	StateCreated State = "created"

	// StateRunning is a VM that is booted, with its task running in it.
	StateRunning State = "running"

	// StateRestarting is a VM on its way back up: its task is being started
	// again by its restart policy, or the VM is being booted again by a
	// restart. A task on its way back up has not ended, and nothing watching
	// it is to think it has.
	StateRestarting State = "restarting"

	// StateExited is a VM whose task ended. Its machine is turned off; its
	// disks and its logs stay until it is deleted, and starting it again
	// boots it from them.
	StateExited State = "exited"

	// StateDead is a VM whose machine went away under its task: the VMM
	// ended, the guest panicked, or the agent was lost.
	StateDead State = "dead"

	// StateRemoving is a VM being deleted.
	StateRemoving State = "removing"
)

// Ended reports whether a VM's task has stopped running for good.
func (s State) Ended() bool {
	return s == StateExited || s == StateDead || s == StateRemoving
}

// Up reports whether a VM's task is running, or on its way up again.
func (s State) Up() bool {
	return s == StateRunning || s == StateRestarting
}

// idPattern is what a VM is called: sixteen hex digits. It is short on
// purpose: a VM's directory, its sockets and its tap devices are named after
// it, a unix socket's path is at most 108 bytes and a device's name at most
// fifteen characters.
var idPattern = regexp.MustCompile(`^[0-9a-f]{16}$`)

// IsID reports whether id can name a VM.
func IsID(id string) bool {
	return idPattern.MatchString(id)
}

// VM is one microVM as vmhost holds it: what it was asked to be, and what it
// has become. It is what vmhost writes down about the VM, and what it answers
// when asked about one.
type VM struct {
	ID   string `json:"id"`
	Spec Spec   `json:"spec"`

	State State `json:"state"`

	// ExitCode is what the task returned, or 128+N when signal N ended it. A
	// task whose machine went away under it is taken to have been killed.
	ExitCode     int  `json:"exit_code"`
	RestartCount uint `json:"restart_count"`

	// Stopped says the task was stopped or killed on purpose, which no
	// restart policy undoes.
	Stopped bool `json:"stopped,omitempty"`

	// Reason is why a VM is dead, or could not be started, when vmhost can
	// say.
	Reason string `json:"reason,omitempty"`

	CreatedAt  time.Time `json:"created_at"`
	StartedAt  time.Time `json:"started_at,omitzero"`
	FinishedAt time.Time `json:"finished_at,omitzero"`

	// ImageDigest is the image the VM boots, as it was when the VM was made:
	// a tag that moves later does not move the VM.
	ImageDigest string `json:"image_digest,omitempty"`

	// Process is what the VM runs, as its image and its spec say it
	// together, the way a container runtime merges them.
	Process guest.Process `json:"process"`

	// VCPUs and MemoryMiB are the machine's size, worked out from the spec's
	// resources: whole CPUs, and memory in MiB, never less than vmhost's
	// minimum.
	VCPUs     int `json:"vcpus"`
	MemoryMiB int `json:"memory_mib"`

	// UID is the host user the VM's VMM runs as: one of its own, which owns
	// its directory, its scratch disk and its taps, and nothing of any other
	// VM's.
	UID int `json:"uid,omitempty"`

	// Interfaces are the VM's network devices while it runs. They are given
	// when it starts and taken back when it stops, since an address is only
	// good while the network that gave it is.
	Interfaces []Interface `json:"interfaces,omitempty"`

	// Generation is which run of the task inside the machine is the current
	// one: a task started again in its machine is a new run of it.
	Generation uint64 `json:"generation,omitempty"`

	// LogBase is what the agent's numbering of this boot's output is counted
	// after. A machine that boots again numbers its output from one again,
	// so what it writes is kept numbered after everything it wrote before.
	LogBase uint64 `json:"log_base,omitempty"`
}

// Endpoints are the ports the VM's task can be reached on right now: every
// port it exposes, while it runs on a network. A VM is reached through its
// agent, so a port is not published anywhere; it is either reachable or not.
func (v VM) Endpoints() []uint16 {
	if v.State != StateRunning || len(v.Interfaces) == 0 {
		return nil
	}

	endpoints := slices.Clone(v.Spec.ExposedPorts)
	slices.Sort(endpoints)

	return slices.Compact(endpoints)
}

// Matches reports whether the VM carries every label filter given, each
// written key=value, which is how a node finds its own VMs, a task's, or the
// ones answering to a slug.
func (v VM) Matches(filters []string) bool {
	for _, filter := range filters {
		key, value, _ := strings.Cut(filter, "=")

		if actual, found := v.Spec.Labels[key]; !found || actual != value {
			return false
		}
	}

	return true
}

// Spec is a VM to make, in the words a task is asked for in.
type Spec struct {
	// Name is what the VM answers to, unique among the VMs vmhost holds, as
	// a container's name is among a daemon's.
	Name string `json:"name"`

	// Hostname is what the VM calls itself. Empty is its ID's beginning.
	Hostname string `json:"hostname,omitempty"`

	// Image is the OCI image the VM runs, as a task names it.
	Image string `json:"image"`

	// Labels are what the VM is for — its task, its node, the slug its ports
	// are served under — kept with the VM so that a node can say what it is
	// holding without asking anything that keeps records.
	Labels map[string]string `json:"labels,omitempty"`

	// Entrypoint, Command, Env and WorkingDir override the image's, the way
	// docker merges them: an entrypoint replaces the image's and its command
	// with it, a command replaces the image's command, and variables are laid
	// over the image's.
	Entrypoint []string `json:"entrypoint,omitempty"`
	Command    []string `json:"command,omitempty"`
	Env        []string `json:"env,omitempty"`
	WorkingDir string   `json:"working_dir,omitempty"`

	Resources Resources `json:"resources"`

	// ReadOnly gives the VM no scratch disk: its root is the image alone,
	// with nothing it can write but tmpfs.
	ReadOnly bool `json:"read_only,omitempty"`

	// RestartPolicy is compose's: "no", "always", "on-failure[:N]" or
	// "unless-stopped". vmhost applies it in place, as docker does.
	RestartPolicy string `json:"restart_policy,omitempty"`

	// AutoRemove deletes the VM once its task has ended.
	AutoRemove bool `json:"auto_remove,omitempty"`

	// Networks are the networks the VM joins, in the order its devices are
	// to find them. None is no network device at all.
	Networks []Attachment `json:"networks,omitempty"`

	// ExposedPorts are the ports the task serves on.
	ExposedPorts []uint16 `json:"exposed_ports,omitempty"`
}

// Resources is what a VM may use. They are the task's own limits, in the
// domain's units, and vmhost turns them into a machine's size.
type Resources struct {
	// CPU is in cores. A machine's CPUs are whole, so a share of one is a
	// whole one, held to the share by a quota where vmhost can set one.
	CPU float64 `json:"cpu"`

	// Memory is in bytes: the machine's memory, rounded up to a MiB and never
	// less than vmhost's minimum. Unlike a container's, it is held for the VM
	// whether it uses it or not.
	Memory uint64 `json:"memory"`

	// Disk is in bytes: the size of the scratch disk the task keeps its
	// changes on, which is the most it can write.
	Disk uint64 `json:"disk"`
}

// Attachment is a network a VM joins, and the names its neighbours on it reach
// it by.
type Attachment struct {
	Network string   `json:"network"`
	Aliases []string `json:"aliases,omitempty"`

	// Gateway marks the network the VM's default route goes through, which
	// is the only one that routes out.
	Gateway bool `json:"gateway,omitempty"`
}

// Interface is one of a running VM's network devices.
type Interface struct {
	Network string `json:"network"`

	// Device is the tap on the host's side.
	Device string `json:"device"`

	// MAC is what the guest finds the device by, and Address the VM's own
	// address on the network, in CIDR form.
	MAC     string `json:"mac"`
	Address string `json:"address"`

	// Gateway is set on the one device the VM routes out through: the
	// network's own address, which the default route goes to.
	Gateway string `json:"gateway,omitempty"`

	// Aliases are the names the VM's neighbours on the network reach it by.
	Aliases []string `json:"aliases,omitempty"`
}
