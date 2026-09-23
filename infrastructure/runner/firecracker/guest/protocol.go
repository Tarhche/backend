// Package guest is what the orchestrator and the agent inside a microVM say to
// each other, and the orchestrator's side of saying it.
//
// The agent is the microVM's init. It listens on a vsock port, which is
// reachable from nothing but the firecracker process holding the machine, and
// from the host only through the unix socket that process exposes. So the
// only party that can speak to it is the orchestrator, and what it says is
// HTTP/1.1 over that connection.
//
// The orchestrator trusts nothing that comes back: a guest runs whatever the
// task put in it, and the agent is only as trustworthy as the machine it runs
// on. Every answer is bounded in size and time.
package guest

import (
	"time"
)

// Port is the vsock port the agent listens on inside every machine.
const Port uint32 = 1024

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
// Numbers are never reused, so a reader that saw up to N asks for what came
// after N, and a gap in the numbers is lines it will never see.
type LogLine struct {
	Seq     uint64    `json:"seq"`
	Stream  string    `json:"stream"`
	At      time.Time `json:"at"`
	Content string    `json:"content"`
}

// Stats is what a machine's task is using.
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

// Exec is a command run inside a machine alongside its task.
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

// The protocols a connection is upgraded to once the agent has agreed to
// what was asked of it.
const (
	// UpgradeExec carries a command's input and output as frames.
	UpgradeExec = "runner-exec"

	// UpgradeDial carries raw bytes to and from one of the task's ports.
	UpgradeDial = "runner-dial"

	// ExecIDHeader names the command an exec connection carries, so it can be
	// ended once nobody is attached to it.
	ExecIDHeader = "X-Runner-Exec-Id"
)
