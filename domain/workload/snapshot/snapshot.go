// Package snapshot is a VM's disk, kept as an archive, as the blog shows one.
//
// What a snapshot is, and everything the workload does with it, is the
// snapshot kind's (domain/workload/kinds/snapshot): the control plane keeps it
// as a manifest. This is the shape the blog's dashboard reads one in, which
// the control plane's client reads a manifest back as, so that the dashboard's
// answers stay what they always were.
package snapshot

import (
	"time"

	"github.com/khanzadimahdi/testproject/domain/workload/vm"
)

// Snapshot is one archive of a VM's disk.
type Snapshot struct {
	UUID      string
	Name      string
	OwnerUUID string

	// VMUUID and VMName are the VM it was taken of, which may be gone by now.
	VMUUID string
	VMName string

	Kind  vm.Kind
	Image string

	// Disk is the disk, in bytes, a VM it is restored onto needs at least.
	Disk uint64

	// Engine is what wrote the archive, as vm.Archive says it; only the same
	// engine can restore it.
	Engine string

	// Size is how many bytes the archive takes where it is kept.
	Size int64

	State  State
	Reason string

	CreatedAt   time.Time
	CompletedAt time.Time
}
