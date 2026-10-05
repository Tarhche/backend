package task

import (
	"context"
	"io"
	"time"

	"github.com/khanzadimahdi/testproject/domain/workload/network"
	"github.com/khanzadimahdi/testproject/domain/workload/port"
)

// Execution is one run of a task, as whatever runs it holds one: what it was
// asked to be, and what it has become. It is a VM of its own today; nothing
// above this line needs to know that.
type Execution struct {
	// ID is what the runtime calls this run, and Name what it answers to
	// there.
	ID   string
	Name string

	// What this is running, as the runtime was told when the run was made. A
	// runtime keeps it alongside the run -- as the labels of its VM -- so that
	// a node can say what it is holding without asking anything that keeps
	// records.
	TaskUUID    string
	TaskName    string
	Slug        string
	Kind        Kind
	NodeName    string
	OwnerUUID   string
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

	// NetworkPolicy is how much of the network the run may reach, which the
	// runtime gives its VM as the network the policy maps to.
	NetworkPolicy network.Policy

	HealthCheck string
	AutoRemove  bool
	Environment []string
	Entrypoint  []string
	Command     []string
	CreatedAt   time.Time

	// StartedAt is when this run last started: the moment its main process
	// did. A runtime says it of every run it lists, as of one it inspects,
	// and it is zero for a run that has not started.
	StartedAt time.Time
	ExitCode  int

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
// thing the domain knows about attaching, so no engine type leaks past here.
type ExecSession interface {
	io.ReadWriteCloser

	// Resize tells the command's terminal how big it now is.
	Resize(ctx context.Context, rows uint, cols uint) error

	// End stops the command, and everything it started, once nobody is
	// attached to it any more.
	//
	// Closing a session releases the stream it ran on, and a runtime whose
	// commands outlive their streams leaves what was running inside the task
	// carrying on, with nothing to show it to and no way back to it. Ending is
	// what is sure to stop it: given a moment to finish on its own, asked to
	// stop, and stopped for good if it will not. A command that has already
	// finished is left alone.
	End(ctx context.Context) error
}

// Runtime is whatever runs the tasks. A node's VM engine does today, each run
// a VM of its own, behind infrastructure/workload/task/vmruntime; whatever
// runs them tomorrow, nothing that asks for a task to be run would have to say
// anything different.
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
