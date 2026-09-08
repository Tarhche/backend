package task

import (
	"context"
	"time"

	"github.com/khanzadimahdi/testproject/domain/runner/network"
	"github.com/khanzadimahdi/testproject/domain/runner/node"
	"github.com/khanzadimahdi/testproject/domain/runner/port"
)

// Task represents a task specification
type Task struct {
	UUID string
	Name string
	Slug string
	Kind Kind

	// StackUUID is the stack this container is a service of, empty for one
	// that stands on its own. ServiceName is what its neighbours in that stack
	// reach it by.
	StackUUID   string
	ServiceName string

	State          State
	Image          string
	AutoRemove     bool
	PortBindings   []port.PortMap
	ExposedPorts   []port.Port
	NetworkPolicy  network.Policy
	Endpoints      []Endpoint
	RestartPolicy  string
	RestartCount   uint
	HealthCheck    string
	AttachStdin    bool
	AttachStdout   bool
	AttachStderr   bool
	Environment    []string
	Command        []string
	Entrypoint     []string
	WorkingDir     string
	Mounts         []Mount
	ResourceLimits ResourceLimits
	NodeName       string
	ContainerID    string
	ContainerLogs  []byte
	OwnerUUID      string
	CreatedAt      time.Time
	StartedAt      time.Time
	FinishedAt     time.Time
}

// Endpoint is an exposed container port as it can actually be reached: the
// worker publishes ContainerPort on Host:HostPort and reports it back, which is
// what the ingress proxies to.
type Endpoint struct {
	ContainerPort port.Port
	Host          string
	HostPort      port.Port
}

// Mount represents a mount point of volume
type Mount struct {
	Source   string
	Target   string
	Type     string
	ReadOnly bool
}

// ResourceLimits represents the resource limits of the container
type ResourceLimits struct {
	Cpu    float64
	Memory uint64
	Disk   uint64
}

// Repository represents a repository of tasks
type Repository interface {
	GetAll(ctx context.Context, offset uint, limit uint) ([]Task, error)
	GetAllByStack(ctx context.Context, stackUUID string) ([]Task, error)
	GetOne(ctx context.Context, UUID string) (Task, error)
	GetOneBySlug(ctx context.Context, slug string) (Task, error)
	Save(ctx context.Context, t *Task) (uuid string, err error)
	Delete(ctx context.Context, UUID string) error
	Count(ctx context.Context) (uint, error)
}

type Scheduler interface {
	Pick(t *Task, candidates []node.Node) node.Node
}
