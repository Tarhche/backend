package task

import (
	"context"
	"io"
	"time"

	"github.com/khanzadimahdi/testproject/domain/runner/network"
	"github.com/khanzadimahdi/testproject/domain/runner/port"
)

// Execution is one run of a task, as whatever runs it holds one: what it was
// asked to be, and what it has become. Docker calls this a container; nothing
// above this line needs to know that.
type Execution struct {
	// ID is what the runtime calls this run, and Name what it answers to
	// there.
	ID   string
	Name string

	// What this is running, as the runtime was told when the run was made. A
	// runtime keeps it alongside the run -- docker as labels -- so that a node
	// can say what it is holding without asking anything that keeps records.
	TaskUUID    string
	TaskName    string
	Slug        string
	Kind        Kind
	NodeName    string
	OwnerUUID   string
	StackUUID   string
	Attempt     int
	Interactive bool

	// TTL is how long this may run for once it is up. Zero is no limit.
	TTL time.Duration

	Status           Status
	Image            string
	ResourceLimits   ResourceLimits
	RestartPolicy    string
	RestartCount     uint
	WorkingDirectory string
	ExposedPorts     port.PortSet
	PortBindings     port.PortMap
	Networks         []network.Attachment
	HealthCheck      string
	AutoRemove       bool
	Environment      []string
	Entrypoint       []string
	Command          []string
	CreatedAt        time.Time
	StartedAt        time.Time
	ExitCode         int

	// ReadOnly makes the task's root filesystem immutable, so nothing it
	// runs can change the image it was started from.
	ReadOnly bool
}

// Deadline is when this task will have run long enough, counted from the
// moment it started. One that may run as long as it likes, or that has not
// started, has none.
func (c *Execution) Deadline() time.Time {
	ttl := c.TTL

	if ttl <= 0 || c.StartedAt.IsZero() {
		return time.Time{}
	}

	return c.StartedAt.Add(ttl)
}

// ExecOptions describes a command to run inside a running task.
type ExecOptions struct {
	Command []string
	TTY     bool
	Env     []string
	WorkDir string
}

// ExecSession is a command running inside a task. Reading takes its
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
	// inside the task carries on, with nothing to show it to and no way
	// back to it. Ending gives it a moment to finish on its own, asks it to
	// stop, and stops it for good if it will not. A command that has already
	// finished is left alone.
	End(ctx context.Context) error
}

// Runtime is whatever runs the tasks. Docker does today, behind
// infrastructure/runner/container; a microvm could tomorrow, and nothing that
// asks for a task to be run would have to say anything different.
type Runtime interface {
	// OnNode is every run the named node is holding, whatever state it is in.
	OnNode(ctx context.Context, nodeName string) ([]Execution, error)

	// Of is the runs of one task. A retry is a new run of the same task, and
	// the one it replaces may still be there.
	Of(ctx context.Context, taskUUID string) ([]Execution, error)

	// BySlug is the runs answering to the name a task's ports are served
	// under.
	BySlug(ctx context.Context, slug string) ([]Execution, error)

	// EnsureImage makes sure an image is on this node, pulling it if it is
	// not. Starting a task does this too; it is worth doing on its own when
	// something is being timed from the moment the task is started.
	EnsureImage(ctx context.Context, image string) error
	Create(ctx context.Context, execution *Execution) (executionID string, err error)
	Start(ctx context.Context, executionID string) error
	Stop(ctx context.Context, executionID string) error
	Restart(ctx context.Context, executionID string) error
	Kill(ctx context.Context, executionID string) error
	Delete(ctx context.Context, executionID string) error
	Inspect(ctx context.Context, executionID string) (Execution, error)
	Stats(ctx context.Context, executionID string) (Stats, error)
	Logs(ctx context.Context, executionID string, writer io.Writer) error
	StreamLogs(ctx context.Context, executionID string, since time.Time, emit func(LogLine) error) error
	Exec(ctx context.Context, executionID string, options ExecOptions) (ExecSession, error)
}
