// Package vm is a user's virtual machine, as the dashboard shows it, and
// the Engine that runs one on a node, with its words for one.
//
// The control plane keeps a VM as the vm kind's manifest
// (domain/workload/kinds/vm): what it was asked to be, what its node last said
// it is, and where it runs, which reads back as a VM here. What actually runs
// it is an Engine on one node, which knows nothing about records and answers
// only for the instances it holds.
package vm

import (
	"strings"
	"time"

	"github.com/khanzadimahdi/testproject/domain/workload/port"
)

// Kind is what a VM boots into, which its image says (KindOf).
type Kind string

const (
	// KindMachine boots an operating system image with no main process: it is
	// a machine somebody opens a terminal in, and it runs until it is stopped.
	KindMachine Kind = "machine"

	// KindDocker boots the Docker image, a docker-in-docker image, with
	// dockerd running, which is what containers and stacks are run in. A VM
	// is a Docker VM because its image is the Docker image; nothing ever looks
	// inside a VM to find a docker daemon.
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

// dockerHub is the registry an image whose name says none is pulled from.
const dockerHub = "docker.io"

// KindOf is what a VM that boots image boots into, where dockerImage is the
// image Docker VMs boot from: a Docker VM when the two name the same
// repository, whatever either is tagged or pinned to, so that a VM made
// before the Docker image moved on to a newer tag is a Docker VM still; and a
// machine otherwise, one that names no image, which boots the default one,
// among them.
//
// It is the one rule for what a VM is. Nothing records it beside the image:
// every service that has to know reads it off the image, with the Docker
// image it is given, which is the same one everywhere.
func KindOf(image string, dockerImage string) Kind {
	if repository := repositoryOf(dockerImage); len(repository) > 0 && repositoryOf(image) == repository {
		return KindDocker
	}

	return KindMachine
}

// repositoryOf is the repository an image names, as a registry knows it:
// without its digest or its tag, and in full on Docker Hub, where
// docker:29-dind is docker.io/library/docker. A tag follows the last colon
// after the last slash, which a registry's port never does, and a name's
// first part is a registry only when it has a dot or a port in it, or is
// localhost. No image names no repository.
func repositoryOf(image string) string {
	name, _, _ := strings.Cut(strings.TrimSpace(image), "@")

	if colon := strings.LastIndex(name, ":"); colon > strings.LastIndex(name, "/") {
		name = name[:colon]
	}

	if len(name) == 0 {
		return ""
	}

	registry, path, qualified := strings.Cut(name, "/")
	if !qualified || (!strings.ContainsAny(registry, ".:") && registry != "localhost") {
		registry, path = dockerHub, name
	}

	if registry == "index."+dockerHub {
		registry = dockerHub
	}

	if registry == dockerHub && !strings.Contains(path, "/") {
		path = "library/" + path
	}

	return registry + "/" + path
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

	// Kind is what it boots into, which its image says (KindOf).
	Kind Kind

	// Image is the OCI reference it boots from. A VM that names none is a
	// machine, and boots the default image; it never changes once the VM
	// exists.
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
