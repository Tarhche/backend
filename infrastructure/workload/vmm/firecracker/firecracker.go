// Package firecracker is vmhost's Hypervisor for Firecracker.
//
// It starts a machine's firecracker as the machine's own host user, links
// what the machine boots from into the machine's own directory, configures
// the machine over its API socket with firecracker-go-sdk — its size, its
// kernel and initramfs, its drives, its network devices and its vsock — and
// boots it. A machine's user owns its directory, its scratch disk and its
// taps, and nothing of any other machine's; firecracker's own seccomp filters
// stay on.
//
// Where a machine's firecracker runs is the Mode. ModeSystemd starts it as a
// transient unit of the host's systemd, over D-Bus, in the slice every machine
// shares: it is then the host's process rather than vmhost's, and outlives
// vmhost's container, which is redeployed with every release. ModeChild starts
// it as vmhost's own child, which ends with vmhost, and is for development.
// Either way the machine runs in the network namespace its taps are in, and a
// vmhost that restarts finds its machines again by their IDs alone.
package firecracker

import (
	"errors"
	"log/slog"

	"github.com/khanzadimahdi/testproject/domain/workload/vm"
)

// Mode is where a machine's firecracker runs.
type Mode string

const (
	// ModeSystemd runs it as a transient host systemd unit, which outlives
	// vmhost.
	ModeSystemd Mode = "systemd"

	// ModeChild runs it as vmhost's own child, which ends with vmhost.
	ModeChild Mode = "child"
)

// Config is how machines' firecrackers are started.
type Config struct {
	// DataDir is vmhost's data directory (infrastructure/workload/vmm/layout),
	// which is the same path on the host: a unit runs what it is given there.
	DataDir string

	// Binary is the firecracker binary, which is copied into the data
	// directory by what it holds before anything is run from it, so an
	// upgrade never replaces one a machine was started from.
	Binary string

	Mode Mode

	// Slice is the systemd slice machines' units run in.
	Slice string

	// NetworkNamespace is the network namespace machines' firecrackers run in,
	// as a path the host's systemd can open. Empty is vmhost's own.
	NetworkNamespace string

	// FirstUID and UIDs are the host users machines run as: the range a
	// machine's user has to be in, and the one a machine left running is
	// found by. UIDs zero runs every machine as vmhost itself, which is for
	// development and nothing else.
	FirstUID int
	UIDs     int

	// MemoryOverhead is what a machine's firecracker is let use beside its
	// guest's memory, in bytes, before its unit is held to it.
	MemoryOverhead uint64
}

// name is what this hypervisor is called.
const name = "firecracker"

// errNotImplemented is what a stub answers.
var errNotImplemented = errors.New("firecracker: not implemented yet")

// Hypervisor boots machines with Firecracker.
type Hypervisor struct {
	config Config
	logger *slog.Logger
}

var _ vm.Hypervisor = (*Hypervisor)(nil)

// Name is the VMM this hypervisor boots machines with.
func (h *Hypervisor) Name() string {
	return name
}
