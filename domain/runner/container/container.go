package container

import (
	"context"
	"io"
	"strconv"
	"time"

	"github.com/khanzadimahdi/testproject/domain/runner/network"
	"github.com/khanzadimahdi/testproject/domain/runner/port"
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

	// ReadOnly makes the container's root filesystem immutable, so nothing it
	// runs can change the image it was started from.
	ReadOnly bool

	ExposedPorts port.PortSet
	PortBindings port.PortMap

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

// Attempt is which attempt at its task this container is, counting from zero,
// as it was labelled when it was created. One from before attempts were
// counted is the first of them.
func (c *Container) Attempt() int {
	attempt, err := strconv.Atoi(c.Labels[TaskAttemptLabelKey])
	if err != nil || attempt < 0 {
		return 0
	}

	return attempt
}

// Interactive reports whether somebody is watching this container run, as it
// was labelled when it was created.
func (c *Container) Interactive() bool {
	return c.Labels[TaskInteractiveLabelKey] == "true"
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

	// End stops the command, and everything it started, once nobody is
	// attached to it any more.
	//
	// Closing a session only releases the stream it ran on: what was running
	// inside the container carries on, with nothing to show it to and no way
	// back to it. Ending gives it a moment to finish on its own, asks it to
	// stop, and stops it for good if it will not. A command that has already
	// finished is left alone.
	End(ctx context.Context) error
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
}

const (
	TaskUUIDLabelKey = "task.uuid" // The UUID of the task that the container is running
	TaskNameLabelKey = "task.name" // The name of the task that the container is running
	TaskSlugLabelKey = "task.slug" // The unique, publicly addressable name of the task
	TaskKindLabelKey = "task.kind" // Whether the task is a one-shot job or a long-running service
	NodeNameLabelKey = "node.name" // the name of the node that manages the container.

	// TaskInteractiveLabelKey marks a container somebody is watching while it
	// runs. It is kept on the container so that whoever reports on it can say
	// so without looking anything up.
	TaskInteractiveLabelKey = "task.interactive"

	// TaskAttemptLabelKey is which attempt this container is, counting from
	// zero. It is kept on the container rather than written down anywhere,
	// because that is exactly how long it means anything: a container that is
	// taken away takes the failures behind it with it.
	TaskAttemptLabelKey = "task.attempt"
)
