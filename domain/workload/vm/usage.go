package vm

import (
	"context"
	"time"
)

// UsageReader reads what a running machine uses from the host's side of it.
//
// The guest says what its task uses too (GuestClient.Stats), and its view of
// memory is the one that means anything, since the host's view of a VM's
// memory only ever grows. But a guest is only as trustworthy as the task in it,
// and a machine's VMM uses CPU of its own that the guest never sees: what the
// host counts — the machine's own cgroup, its taps — is what the machine costs
// the host, and what vmhost reports where it can count it.
type UsageReader interface {
	// Usage is what a machine has used, all told, since it started. Devices
	// are its taps on the host's side.
	Usage(ctx context.Context, machine Machine, devices []string) (Usage, error)
}

// Usage is what a machine has used, all told, as the host counts it.
type Usage struct {
	// CPU is the CPU time its cgroup has had, its VMM's included.
	CPU time.Duration

	// NetworkInput is what its taps carried to it, and NetworkOutput what
	// they carried from it, in bytes.
	NetworkInput  uint64
	NetworkOutput uint64

	// BlockInput and BlockOutput are what its cgroup read and wrote, in
	// bytes.
	BlockInput  uint64
	BlockOutput uint64
}
