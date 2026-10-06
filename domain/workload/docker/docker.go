// Package docker is what runs inside one Docker VM: its containers, images,
// networks and volumes, and the compose projects deployed into it.
//
// None of it is stored by the workload. dockerd is the truth about its own
// objects, so they are read from it live and never kept anywhere else. These
// are value types with no wire shape of their own: what travels between the
// workload's services has its own types, next to the messages that carry it.
package docker

import (
	"context"
	"errors"
	"time"

	"github.com/khanzadimahdi/testproject/domain/workload/port"
)

// The labels compose writes on what it creates, which are how a stack's
// containers are told from the rest: a stack's slug is its compose project.
const (
	LabelComposeProject = "com.docker.compose.project"
	LabelComposeService = "com.docker.compose.service"
)

// LabelVM marks an instance a node's engine holds as a Docker VM, with
// "true": it is how a node tells, from its engine alone, which of its VMs
// have a dockerd it reads, since nothing ever looks inside a VM to find one.
const LabelVM = "workload.docker"

var (
	// ErrUnavailable is a dockerd that did not answer, even after it was given
	// time to come up with its VM.
	ErrUnavailable = errors.New("docker is not available")

	// ErrInvalid is a request dockerd refused as it stood: an image that does
	// not exist, a name already taken, a network still in use. The error that
	// wraps it carries docker's own words.
	ErrInvalid = errors.New("docker refused the request")
)

// Container is one container in a Docker VM.
type Container struct {
	ID    string
	Name  string
	Image string

	// State is docker's: created, running, paused, restarting, removing,
	// exited or dead. Status is the sentence docker puts it in.
	State  string
	Status string

	Command  string
	Ports    []PortBinding
	Networks []string
	Mounts   []Mount
	Labels   map[string]string

	// Stack and Service are the compose project and service the container
	// belongs to, when it belongs to one.
	Stack   string
	Service string

	RestartPolicy string
	CreatedAt     time.Time
}

// PortBinding is a container port published on the Docker VM.
type PortBinding struct {
	ContainerPort port.Port
	HostPort      port.Port

	// Protocol is tcp or udp.
	Protocol string
}

// Mount is something mounted into a container.
type Mount struct {
	// Type is volume, bind or tmpfs.
	Type     string
	Source   string
	Target   string
	ReadOnly bool
}

// ContainerSpec is a container to create.
type ContainerSpec struct {
	Name  string
	Image string

	Command    []string
	Entrypoint []string
	Env        []string
	WorkingDir string

	Ports    []PortBinding
	Mounts   []Mount
	Networks []string

	// RestartPolicy is no, always, unless-stopped or on-failure.
	RestartPolicy string

	// CPUs is in cores and Memory in bytes; zero is no limit.
	CPUs   float64
	Memory uint64

	Labels map[string]string
}

// ContainerFilter narrows a listing of containers.
type ContainerFilter struct {
	// All includes the containers that are not running.
	All bool

	// Stack keeps only the containers of one compose project.
	Stack string

	// Label keeps only the containers that carry a label, written as its
	// key, whatever its value, or as key=value.
	Label string
}

// Image is one image a Docker VM holds.
type Image struct {
	ID   string
	Tags []string

	// Size is in bytes.
	Size int64

	CreatedAt time.Time

	// InUse says a container was created from it.
	InUse bool
}

// Network is one docker network in a Docker VM. It never reaches past the VM.
type Network struct {
	ID       string
	Name     string
	Driver   string
	Scope    string
	Internal bool

	// Containers are the containers attached to it.
	Containers []string

	Labels    map[string]string
	CreatedAt time.Time
}

// NetworkSpec is a network to create.
type NetworkSpec struct {
	Name     string
	Driver   string
	Internal bool
	Labels   map[string]string
}

// Volume is one docker volume in a Docker VM.
type Volume struct {
	Name       string
	Driver     string
	Mountpoint string
	Labels     map[string]string

	// InUse says a container mounts it.
	InUse bool

	CreatedAt time.Time
}

// VolumeSpec is a volume to create.
type VolumeSpec struct {
	Name   string
	Driver string
	Labels map[string]string
}

// Stats is one sample of what a container is using. Memory, network and block
// counters are bytes.
type Stats struct {
	// CPUPercent is how busy the container kept the CPUs it can see, which are
	// its VM's vCPUs, as a share of all of them together: 0 to 100, where
	// docker's own figure goes up to 100 for each CPU. It means what a VM's
	// does, so the two can be read side by side.
	CPUPercent  float64
	MemoryUsed  uint64
	MemoryLimit uint64
	NetworkRx   uint64
	NetworkTx   uint64
	BlockRead   uint64
	BlockWrite  uint64
	PIDs        uint64
	SampledAt   time.Time
}

// LogOptions narrow what is read of a container's log.
type LogOptions struct {
	// Since leaves out the lines written before it; zero is from the start.
	Since time.Time

	// Tail keeps only the last lines; zero is all of them.
	Tail uint
}

// LogLine is one line a container wrote.
type LogLine struct {
	At time.Time

	// Stream is stdout or stderr.
	Stream string

	Line string
}

// Daemon is the dockerd of one Docker VM.
type Daemon interface {
	// Ping answers once dockerd does.
	Ping(ctx context.Context) error

	Containers(ctx context.Context, filter ContainerFilter) ([]Container, error)
	Container(ctx context.Context, id string) (Container, error)

	// CreateContainer pulls the image when the VM does not hold it, then
	// creates the container and starts it.
	CreateContainer(ctx context.Context, spec ContainerSpec) (Container, error)

	StartContainer(ctx context.Context, id string) error
	StopContainer(ctx context.Context, id string) error
	RestartContainer(ctx context.Context, id string) error
	RemoveContainer(ctx context.Context, id string, force bool) error
	ContainerLogs(ctx context.Context, id string, options LogOptions) ([]LogLine, error)
	ContainerStats(ctx context.Context, id string) (Stats, error)

	// ConnectNetwork attaches a container to a network, under the aliases its
	// neighbours there reach it by.
	ConnectNetwork(ctx context.Context, network string, container string, aliases []string) error
	DisconnectNetwork(ctx context.Context, network string, container string, force bool) error

	Images(ctx context.Context) ([]Image, error)
	PullImage(ctx context.Context, reference string) (Image, error)
	RemoveImage(ctx context.Context, id string, force bool) error

	Networks(ctx context.Context) ([]Network, error)
	CreateNetwork(ctx context.Context, spec NetworkSpec) (Network, error)
	RemoveNetwork(ctx context.Context, id string) error

	Volumes(ctx context.Context) ([]Volume, error)
	CreateVolume(ctx context.Context, spec VolumeSpec) (Volume, error)
	RemoveVolume(ctx context.Context, name string, force bool) error
}
