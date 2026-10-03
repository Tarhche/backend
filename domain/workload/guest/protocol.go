// Package guest is what vmhost and the agent inside a microVM say to each
// other.
//
// The agent is the microVM's init. It listens on a vsock port, which is
// reachable from nothing but the firecracker process holding the machine, and
// from the host only through the unix socket that process exposes: a
// connection to it says "CONNECT 1024" and becomes a connection to the agent.
// So the only party that can speak to it is vmhost, and what it says is
// HTTP/1.1 over that connection. A machine therefore needs no network device
// to be managed, which is what lets a task with no network have none.
//
// vmhost trusts nothing that comes back: a guest runs whatever the task put
// in it, and the agent is only as trustworthy as the machine it runs on. Every
// answer is bounded in size and time, and every frame in length.
//
// The shapes are the ones PR #101's runner-guest spoke, end-to-end tested
// there, renamed for the workload.
package guest

import (
	"errors"
	"time"
)

// Port is the vsock port the agent listens on inside every machine.
const Port uint32 = 1024

// CID is every machine's own end of its vsock. Every machine has the same one:
// the host reaches each through a socket of its own, so the number never has
// to tell one machine from another.
const CID uint32 = 3

// ProtocolVersion is the version of what is said here. The agent answers
// every request with it in VersionHeader. A machine outlives the vmhost that
// booted it, so the vmhost that adopts it may be newer than its agent: vmhost
// refuses to drive an agent whose version it does not know, rather than
// misreading it.
const ProtocolVersion = "1"

// VersionHeader carries ProtocolVersion on every answer.
const VersionHeader = "X-Workload-Guest-Version"

// KernelArgs is the guest kernel's command line. The agent is /init in the
// initramfs; the root its task runs in is put together from the machine's
// disks once the machine is told what it is, so the kernel is given no root.
// A machine asked to reboot ends instead, which is how a machine is turned
// off: firecracker has no power button. Nothing secret is ever put here, since
// the host's process list shows it.
const KernelArgs = "console=ttyS0 reboot=k panic=1 pci=off quiet loglevel=3 rdinit=/init"

// The devices a machine finds its disks as, in the order vmhost attaches
// them: the image first, then the scratch disk a writable task has.
const (
	ImageDevice   = "/dev/vda"
	ScratchDevice = "/dev/vdb"
)

// The agent's routes. Methods and paths are written as Go's ServeMux takes
// them; the agent serves them, and vmhost asks them.
const (
	RouteHealth   = "GET /health"
	RouteConfig   = "PUT /config"
	RouteHosts    = "PUT /hosts"
	RouteStart    = "POST /process"
	RouteStatus   = "GET /process"
	RouteWait     = "GET /process/wait"
	RouteSignal   = "POST /process/signal"
	RouteStop     = "POST /process/stop"
	RouteLogs     = "GET /logs"
	RouteStats    = "GET /stats"
	RouteExec     = "POST /exec"
	RouteEndExec  = "POST /exec/{id}/end"
	RouteDial     = "POST /dial"
	RoutePowerOff = "POST /poweroff"
)

// The query parameters of the routes that take them.
const (
	// QueryGeneration names the run GET /process/wait waits for.
	QueryGeneration = "generation"

	// QueryAfter is the last line number GET /logs is not to send again,
	// and QueryFollow, set to "1", keeps it sending what comes.
	QueryAfter  = "after"
	QueryFollow = "follow"

	// QueryPort is the task's port POST /dial connects to.
	QueryPort = "port"
)

// The protocols a connection is upgraded to once the agent has agreed to
// what was asked of it.
const (
	// UpgradeExec carries a command's input and output as frames.
	UpgradeExec = "workload-exec"

	// UpgradeDial carries raw bytes to and from one of the task's ports.
	UpgradeDial = "workload-dial"

	// ExecIDHeader names the command an exec connection carries, so it can be
	// ended once nobody is attached to it.
	ExecIDHeader = "X-Workload-Exec-Id"
)

// ErrNotRunning is the agent refusing what only a running task can do. The
// agent says it with 409 Conflict.
var ErrNotRunning = errors.New("the task is not running")

// Config is what a machine is told once, after it boots and before its task
// runs: which disks hold its root, which interfaces it has, and what it is
// called.
type Config struct {
	// Now is the host's clock, which the guest takes as its own when the
	// two disagree: a task's log lines are stamped inside the guest.
	Now time.Time `json:"now"`

	Hostname string `json:"hostname"`
	Root     Root   `json:"root"`

	// Interfaces are matched to the machine's network devices by MAC.
	Interfaces []Interface `json:"interfaces,omitempty"`

	// Nameservers answer names nothing on the machine's own networks does.
	// A machine that cannot reach the internet is given none.
	Nameservers []string `json:"nameservers,omitempty"`

	// Hosts are the names the machine's neighbours answer to, which is how
	// the services of a stack reach each other by service name.
	Hosts []Host `json:"hosts,omitempty"`
}

// Root is the disks a machine's root filesystem is made of.
type Root struct {
	// Image is the device holding the image the task runs, always read only:
	// one image serves every machine that runs it.
	Image string `json:"image"`

	// Scratch is the device a writable root keeps its changes on, overlaid
	// on Image. A read-only task has none, and can change nothing.
	Scratch string `json:"scratch,omitempty"`
}

// ReadOnly reports whether the root cannot be written to at all.
func (r Root) ReadOnly() bool {
	return len(r.Scratch) == 0
}

// Interface is one of a machine's network devices.
type Interface struct {
	MAC string `json:"mac"`

	// Address is the interface's own address, in CIDR form.
	Address string `json:"address"`

	// Gateway is set on the one interface a machine's default route goes
	// through, and on no other.
	Gateway string `json:"gateway,omitempty"`
}

// Host is an address and the names it answers to.
type Host struct {
	Address string   `json:"address"`
	Names   []string `json:"names"`
}

// Process is what a machine runs: its task, or a command inside it.
type Process struct {
	// Args are the command and its arguments, as resolved from the image and
	// the task together. The command is looked up on the root's own PATH.
	Args []string `json:"args"`
	Env  []string `json:"env,omitempty"`

	// WorkingDir is where it starts, inside the root. Empty is the root.
	WorkingDir string `json:"working_dir,omitempty"`

	// User is who it runs as: a name or uid, optionally with a group, as an
	// image says it. Empty is root.
	User string `json:"user,omitempty"`
}

// The states a machine's task passes through.
const (
	StateCreated = "created"
	StateRunning = "running"
	StateExited  = "exited"
)

// Status is what has become of a machine's task.
type Status struct {
	State string `json:"state"`

	// Generation counts the times the task has been started in this
	// machine, so that waiting for one run to end is not answered by the end
	// of the run before it.
	Generation uint64 `json:"generation"`

	StartedAt  time.Time `json:"started_at,omitzero"`
	FinishedAt time.Time `json:"finished_at,omitzero"`

	// ExitCode is what the task returned, or 128+N when signal N ended it.
	ExitCode int `json:"exit_code"`
}

// The streams a line of output comes from.
const (
	StreamStdout = "stdout"
	StreamStderr = "stderr"
)

// LogLine is one line a task wrote, numbered in the order it was written.
// Numbers are never reused within a boot, so a reader that saw up to N asks
// for what came after N, and a gap in the numbers is lines it will never see.
// A machine that boots again numbers from one again.
type LogLine struct {
	Seq     uint64    `json:"seq"`
	Stream  string    `json:"stream"`
	At      time.Time `json:"at"`
	Content string    `json:"content"`
}

// Stats is what a machine's task is using, as the guest sees it.
type Stats struct {
	PIDs uint64 `json:"pids"`

	// CPUPercent is the share of one CPU the task used since the last time
	// it was asked, so a task busy on two CPUs reports 200.
	CPUPercent float64 `json:"cpu_percent"`

	MemoryUsage uint64 `json:"memory_usage"`
	MemoryLimit uint64 `json:"memory_limit"`

	NetworkInput  uint64 `json:"network_input"`
	NetworkOutput uint64 `json:"network_output"`
	BlockInput    uint64 `json:"block_input"`
	BlockOutput   uint64 `json:"block_output"`
}

// Stop asks a task to end: it is signalled, given Timeout to go on its own,
// and then ended along with everything it started.
type Stop struct {
	Timeout time.Duration `json:"timeout"`
}

// Signal is sent to a task's own process.
type Signal struct {
	Signal int `json:"signal"`
}

// Exec is a command run inside a machine alongside its task. One that names no
// user or working directory runs as the task does, with the task's
// environment under its own.
type Exec struct {
	Process

	TTY  bool   `json:"tty"`
	Rows uint16 `json:"rows,omitempty"`
	Cols uint16 `json:"cols,omitempty"`
}

// EndExec is how a command left without anybody attached to it is ended: it
// is given Grace to finish on its own, asked to stop, and given KillGrace
// before it is stopped for good.
type EndExec struct {
	Grace     time.Duration `json:"grace"`
	KillGrace time.Duration `json:"kill_grace"`
}

// Ended reports whether anything was still there to end.
type Ended struct {
	Signalled bool `json:"signalled"`
}
