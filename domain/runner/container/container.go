package container

import (
	"context"
	"io"
	"time"

	"github.com/khanzadimahdi/testproject/domain/runner/network"
	"github.com/khanzadimahdi/testproject/domain/runner/port"
	"github.com/khanzadimahdi/testproject/domain/runner/task"
)

// Container represents a container specification
type Container struct {
	ID               string
	Name             string
	Status           Status
	Image            string
	ResourceLimits   ResourceLimits
	RestartPolicy    string
	RestartCount     uint
	WorkingDirectory string
	ExposedPorts     port.PortSet
	PortBindings     port.PortMap

	// Networks are the networks the container joins, in order. The first is
	// the one it is created on; the rest it is connected to afterwards, which
	// is how a public container reaches both its own stack and the internet.
	Networks []network.Attachment

	HealthCheck string
	AutoRemove  bool
	Environment []string
	Entrypoint  []string
	Command     []string
	Labels      map[string]string
	CreatedAt   time.Time
}

// ResourceLimits represents the resource limits of the container
type ResourceLimits struct {
	Cpu    float64
	Memory uint64
	Disk   uint64
}

// ExecOptions describes a command to run inside a running container.
type ExecOptions struct {
	Command []string
	TTY     bool
	Env     []string
	WorkDir string
}

// ExecSession is a command running inside a container. Reading takes its
// output, writing feeds its input, and closing tears it down. It is the only
// thing the domain knows about attaching, so no docker type leaks past here.
type ExecSession interface {
	io.ReadWriteCloser

	// Resize tells the command's terminal how big it now is.
	Resize(ctx context.Context, rows uint, cols uint) error
}

// Manager represents a manager of containers
type Manager interface {
	GetAll(ctx context.Context) ([]Container, error)
	GetByLabel(ctx context.Context, labelName string, labelValue string) ([]Container, error)
	Create(ctx context.Context, container *Container) (containerUUID string, err error)
	Start(ctx context.Context, containerUUID string) error
	Stop(ctx context.Context, containerUUID string) error
	Restart(ctx context.Context, containerUUID string) error
	Kill(ctx context.Context, containerUUID string) error
	Delete(ctx context.Context, containerUUID string) error
	Inspect(ctx context.Context, containerUUID string) (Container, error)
	Stats(ctx context.Context, containerUUID string) (Stats, error)
	Logs(ctx context.Context, containerUUID string, writer io.Writer) error
	StreamLogs(ctx context.Context, containerUUID string, since time.Time, emit func(LogLine) error) error
	Exec(ctx context.Context, containerUUID string, options ExecOptions) (ExecSession, error)
	EvaluateTaskState(status Status) task.State
}

const (
	TaskUUIDLabelKey = "task.uuid" // The UUID of the task that the container is running
	TaskNameLabelKey = "task.name" // The name of the task that the container is running
	TaskSlugLabelKey = "task.slug" // The unique, publicly addressable name of the task
	TaskKindLabelKey = "task.kind" // Whether the task is a one-shot job or a long-running service
	NodeNameLabelKey = "node.name" // the name of the node that manages the container.
)
