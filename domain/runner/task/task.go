package task

import (
	"context"
	"time"

	"github.com/khanzadimahdi/testproject/domain/runner/node"
	"github.com/khanzadimahdi/testproject/domain/runner/port"
)

// Task represents a task specification
type Task struct {
	UUID           string
	Name           string
	State          State
	Image          string
	AutoRemove     bool
	PortBindings   []port.PortMap
	RestartPolicy  string
	RestartCount   uint
	HealthCheck    string
	AttachStdin    bool
	AttachStdout   bool
	AttachStderr   bool
	Environment    []string
	Command        []string
	Entrypoint     []string
	Mounts         []Mount
	ResourceLimits ResourceLimits
	ContainerID    string
	ContainerLogs  []byte
	OwnerUUID      string
	CreatedAt      time.Time
	StartedAt      time.Time
	FinishedAt     time.Time
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
	GetOne(ctx context.Context, UUID string) (Task, error)
	Save(ctx context.Context, t *Task) (uuid string, err error)
	Delete(ctx context.Context, UUID string) error
	Count(ctx context.Context) (uint, error)
}

type Scheduler interface {
	Pick(t *Task, candidates []node.Node) node.Node
}
