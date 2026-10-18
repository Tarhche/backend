package vm

import "errors"

var (
	// ErrEngineMismatch is an archive from another engine, or from another
	// version of this one, which a restore cannot read.
	ErrEngineMismatch = errors.New("the archive was written by another engine")

	// ErrNoCapacity is a node that has no room left for what was asked of it.
	ErrNoCapacity = errors.New("no capacity")

	// ErrNotRunning is something only a running VM can do, asked of one that is
	// not running.
	ErrNotRunning = errors.New("the vm is not running")

	// ErrNotDocker is something only a Docker VM can do, asked of a VM of
	// another kind.
	ErrNotDocker = errors.New("the vm is not a docker vm")

	// ErrQuotaExceeded is asking for more than one person is allowed: more VMs,
	// or more of what they are given, than their quota.
	ErrQuotaExceeded = errors.New("quota exceeded")
)
