package runs

import (
	"context"
	"io"
	"syscall"
	"time"
)

// Sandboxes is microsandbox as the run supervisor uses it: one detached
// sandbox for each run, booted when the run starts and stopped when its main
// process ends.
//
// Sizes are bytes and CPUs are cores, as everywhere else in the workload. The
// adapter behind this converts them to microsandbox's MiB and whole vCPUs at
// the edge, and nothing else converts them anywhere.
//
// Nothing here should be cancelled because whoever asked went away: cancelling
// microsandbox's create leaves a stopped sandbox behind. So the supervisor
// calls these with deadlines of its own.
type Sandboxes interface {
	// Check is whether microsandbox can run anything here: /dev/kvm opens
	// for reading and writing, and the runtime's msb and libkrunfw resolve.
	// It reports the SDK's version and the runtime's, which the supervisor
	// runs nothing on unless they are equal, and the architecture guests
	// run.
	Check(ctx context.Context) (Versions, error)

	// Pull makes sure an image is in microsandbox's cache, as msb pull does,
	// and does nothing when it already is.
	Pull(ctx context.Context, reference string) error

	// Image is the configuration of a cached image, and false when the
	// image is not cached.
	Image(ctx context.Context, reference string) (ImageConfig, bool, error)

	// Create makes a run's sandbox and boots it for the first time,
	// detached, so that it outlives the process that made it.
	Create(ctx context.Context, spec SandboxSpec) (Sandbox, error)

	// Start boots a stopped sandbox, detached.
	Start(ctx context.Context, name string) (Sandbox, error)

	// Connect takes a handle on a sandbox that is already running.
	Connect(ctx context.Context, name string) (Sandbox, error)

	// Stop shuts a sandbox down, giving it timeout to go gracefully and
	// killing it after that, since microsandbox never kills one by itself.
	Stop(ctx context.Context, name string, timeout time.Duration) error

	// Remove destroys a sandbox. One that is already gone is no error.
	Remove(ctx context.Context, name string) error

	// List is every sandbox carrying all of the given labels, running or
	// not.
	List(ctx context.Context, labels map[string]string) ([]SandboxInfo, error)

	// Metrics is what each running sandbox is using, keyed by its name, read
	// for all of them at once rather than one at a time.
	Metrics(ctx context.Context) (map[string]Metrics, error)
}

// Sandbox is a live handle on a running sandbox, and the connection to its
// guest agent that comes with it.
//
// Closing it ends every command started through it, because microsandbox kills
// the commands of a client that disconnects. That is why the supervisor holds
// a run's handle for as long as the run's main process runs, and why a main
// process cannot outlive the supervisor.
type Sandbox interface {
	Name() string

	// Exec starts a command in the guest. ctx bounds starting it, not
	// running it.
	Exec(ctx context.Context, command Command) (Process, error)

	Close() error
}

// Process is a command running in a guest.
type Process interface {
	// Events is what becomes of the command, in the order it happens. The
	// last event is always one of EventExited, EventFailed or EventLost, and
	// the channel is closed after it.
	Events() <-chan Event

	// Stdin is the command's standard input, and nil unless the command
	// asked for one.
	Stdin() io.WriteCloser

	// Signal sends a signal to the command's process group, numbered as
	// Linux numbers it, since the guest is Linux.
	Signal(ctx context.Context, signal syscall.Signal) error

	// Resize changes the size of the command's terminal.
	Resize(ctx context.Context, rows, cols uint16) error

	// Close lets go of the handle. The command carries on.
	Close() error
}

// Versions is what Check found.
type Versions struct {
	SDK, Runtime string // the SDK compiled in, and the msb it drives
	Architecture string // the GOARCH every guest runs
}

// ImageConfig is what an image says about how it is run.
type ImageConfig struct {
	// Entrypoint and Cmd are what the supervisor works a run's program out
	// from, by docker's rules rather than microsandbox's.
	Entrypoint, Cmd, Env []string
	WorkingDir, User     string

	// StopSignal is the signal the image asks to be stopped with, by name or
	// by number. It is empty when the image names none, which is SIGTERM.
	StopSignal string

	// Architecture is the GOARCH the image was built for.
	Architecture string
}

// SandboxSpec is what a run's sandbox is made with, once the supervisor has
// applied its own rules: the memory raised to its floor, and a host port
// picked for each published one.
type SandboxSpec struct {
	Name, Image string

	// CPUs is in cores. Microsandbox gives whole vCPUs, so the adapter boots
	// ceil(CPUs) of them, and at least one.
	CPUs float64

	// Memory and Disk are bytes, which the adapter rounds up to whole MiB, so
	// a run never gets less than it asked for. A Disk of zero leaves the
	// writable root at microsandbox's own size.
	Memory, Disk uint64

	// Env is laid over the image's environment. Labels are what match a
	// sandbox to the supervisor's record of its run, and what find a sandbox
	// that has no record.
	Env, Labels map[string]string
	Workdir     string

	Network string // "isolated" or "public"
	Ports   []PortBinding

	// Nameservers are the guest's DNS upstreams under "public", so a guest
	// never asks docker's resolver, which would tell it the platform's own
	// service names. Under "isolated" there is no DNS at all.
	Nameservers []string

	// MaxTCPConnections bounds the TCP connections the guest holds at once,
	// so one run cannot use up the host's.
	MaxTCPConnections int
}

// PortBinding publishes a guest's TCP port on the host. Bind is the address it
// is published on, which has to be one the orchestrators reach, since
// microsandbox binds 127.0.0.1 unless it is told otherwise.
type PortBinding struct {
	Bind                string
	HostPort, GuestPort uint16
}

// SandboxInfo is a sandbox as a listing finds it.
type SandboxInfo struct {
	Name    string
	Labels  map[string]string
	Running bool
}

// Metrics is what a running sandbox is using. Sizes and counters are bytes.
type Metrics struct {
	CPUPercent                        float64
	MemoryUsage, MemoryLimit          uint64
	NetRx, NetTx, DiskRead, DiskWrite uint64
}

// Command is what to run in a guest. It runs as the image's user, in the
// sandbox's environment and working directory unless it says otherwise.
type Command struct {
	// Argv is the program and its arguments, already worked out: nothing
	// falls back to the image's entrypoint or command.
	Argv []string

	Env     []string // KEY=VALUE, added to the sandbox's
	Workdir string   // the sandbox's when empty

	// TTY gives the command a terminal, Rows by Cols to begin with, which
	// makes its stdout and stderr one stream. Stdin gives it a standard
	// input; without it, it has none.
	TTY, Stdin bool
	Rows, Cols uint16
}

// EventKind is what an Event says happened.
type EventKind int

const (
	EventStarted EventKind = iota // the command is running
	EventStdout                   // Data is what it wrote to stdout
	EventStderr                   // Data is what it wrote to stderr
	EventExited                   // ExitCode is what it returned; -1 when a signal ended it
	EventFailed                   // it never started: Errno and Message say why
	EventLost                     // its stream ended without an exit, with its VM or its connection
)

// Event is one thing that happened to a Process.
type Event struct {
	Kind EventKind
	Data []byte

	// ExitCode is microsandbox's, not docker's: a process that a signal ended
	// reports -1, and the supervisor, which knows what it sent, works out the
	// code docker would have reported.
	ExitCode int

	// Errno names the error that kept the command from starting, such as
	// ENOENT, and Message describes it.
	Errno   string
	Message string
}
