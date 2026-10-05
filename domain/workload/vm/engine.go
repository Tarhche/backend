package vm

import (
	"context"
	"io"
	"time"

	"github.com/khanzadimahdi/testproject/domain/workload/port"
)

// Engine runs VMs on one node.
//
// It is the only thing an engine implements: microsandbox does today, and
// another engine would be a second implementation of this, not a second way
// of asking for a VM. An engine knows instances, not records: what it holds is
// named by Spec.ID and described by the labels it was given, so a node can say
// what it is holding without asking anything that keeps records.
type Engine interface {
	// Info is what this node offers to VMs and how much of it is taken.
	Info(ctx context.Context) (Info, error)

	// List is every instance this engine holds, whatever state it is in.
	List(ctx context.Context) ([]Instance, error)

	// Inspect is one instance, or domain.ErrNotExists when there is none.
	Inspect(ctx context.Context, id string) (Instance, error)

	// Create creates an instance and boots it.
	Create(ctx context.Context, spec Spec) (Instance, error)

	Start(ctx context.Context, id string) error
	Stop(ctx context.Context, id string) error
	Restart(ctx context.Context, id string) error

	// Delete stops an instance and removes it, disk and all. Deleting one that
	// is not there is the outcome asked for.
	Delete(ctx context.Context, id string) error

	// Reconfigure applies a spec's ports, network and resources to the
	// instance it names, restarting it when the engine cannot apply them to a
	// running one.
	Reconfigure(ctx context.Context, spec Spec) (Instance, error)

	Stats(ctx context.Context, id string) (Stats, error)
	Logs(ctx context.Context, id string, options LogOptions) ([]LogLine, error)

	// Exec runs a command inside a running instance.
	Exec(ctx context.Context, id string, options ExecOptions) (ExecSession, error)

	// Snapshot writes an instance's disk to archive, image included, so the
	// archive is all a restore needs.
	Snapshot(ctx context.Context, id string, archive io.Writer) (Archive, error)

	// Restore replaces the instance spec.ID names, or creates it, from an
	// archive Snapshot wrote. An archive from another engine is refused with
	// ErrEngineMismatch.
	Restore(ctx context.Context, spec Spec, archive io.Reader) (Instance, error)
}

// Spec is an instance to create, reconfigure or restore.
type Spec struct {
	// ID is the engine's name for the instance: the VM's uuid, or the
	// execution's for a code-runner task.
	ID string

	Kind      Kind
	Image     string
	Resources Resources
	Ports     []port.Port
	Network   Network

	PersistentDisk bool

	// Labels describe what the instance is, under the Label keys, so that
	// listing an engine's instances says what each of them is for and whose.
	Labels map[string]string

	// Entrypoint and Command, when either is set, make up the instance's main
	// process, the way a container's are: it runs once the instance boots, its
	// output goes to the logs, and the instance stops when it exits, keeping its
	// exit code. A nil Entrypoint keeps the image's own, so a Command alone is
	// handed to it as arguments, which is what a code runner's image expects.
	// Naming an Entrypoint drops the image's CMD along with its ENTRYPOINT, as
	// docker does, so the image's CMD is kept only when neither is replaced.
	// Only code-runner tasks set either, so a VM somebody asked for has no main
	// process at all.
	Entrypoint []string
	Command    []string
	Env        []string
	WorkingDir string
}

// HasMainProcess reports whether the instance runs a main process of its own,
// rather than staying up until it is stopped.
func (s Spec) HasMainProcess() bool {
	return len(s.Entrypoint) > 0 || len(s.Command) > 0
}

// InstanceState is what an engine says one of its instances is doing.
type InstanceState string

const (
	InstanceCreated InstanceState = "created"
	InstanceRunning InstanceState = "running"
	InstanceStopped InstanceState = "stopped"

	// InstanceExited is an instance whose main process has ended. Only an
	// instance with a Command has one.
	InstanceExited InstanceState = "exited"

	InstanceFailed InstanceState = "failed"
)

// Instance is the engine's view of one VM, or of one code-runner execution, on
// one node.
type Instance struct {
	ID     string
	State  InstanceState
	Labels map[string]string

	// Endpoints are where this node reaches each published guest port.
	Endpoints []Endpoint

	// ExitCode is how the main process ended, when State is InstanceExited.
	ExitCode int

	Reason    string
	StartedAt time.Time
}

// Endpoint is a guest port and the address this node dials to reach it.
type Endpoint struct {
	Port port.Port

	// Address is host:port, as the orchestrator holding the instance dials it.
	Address string
}

// Info is what a node offers to VMs, and how much of it the instances it holds
// have been given.
type Info struct {
	Engine  string
	Version string

	// CPUs, Memory and Disk are the budget: whole vCPUs, and bytes.
	CPUs   uint
	Memory uint64
	Disk   uint64

	// Allocated is the sum of what the instances this node holds were given.
	Allocated Resources
}

// LogOptions narrow what is read of a log.
type LogOptions struct {
	// Since leaves out the lines written before it; zero is from the start.
	Since time.Time

	// Tail keeps only the last lines; zero is all of them.
	Tail uint
}

// Where a log line came from.
const (
	LogSourceKernel  = "kernel"
	LogSourceRuntime = "runtime"
	LogSourceMain    = "main"
	LogSourceExec    = "exec"
)

// LogLine is one line of an instance's log.
type LogLine struct {
	At time.Time

	// Source is one of the LogSource values.
	Source string

	Line string
}

// ExecOptions describe a command to run inside an instance.
type ExecOptions struct {
	Command []string

	// TTY runs the command on a terminal, which is what an interactive shell
	// needs. Rows and Cols are its size to begin with.
	TTY  bool
	Rows uint
	Cols uint

	Env        []string
	WorkingDir string
}

// ExecSession is a command running inside an instance. It is the only thing
// the domain knows about reaching into one, so no engine type leaks past here.
type ExecSession interface {
	// Stdin is the command's input; closing it is the end of the input.
	Stdin() io.WriteCloser

	// Stdout is the command's output. Under a TTY everything is here.
	Stdout() io.Reader

	// Stderr is the command's errors, and is empty under a TTY.
	Stderr() io.Reader

	// Resize tells the command's terminal how big it now is.
	Resize(ctx context.Context, rows uint, cols uint) error

	// Wait waits for the command to end and says how it did.
	Wait(ctx context.Context) (exitCode int, err error)

	// Close ends the command.
	Close() error
}

// Archive describes what Snapshot wrote.
type Archive struct {
	// Engine is the engine and version that wrote it, such as
	// "microsandbox/0.7.6". A restore needs the same engine.
	Engine string

	Kind  Kind
	Image string

	// Disk is the disk, in bytes, a restore needs at least.
	Disk uint64

	// Size is how many bytes were written.
	Size int64
}
