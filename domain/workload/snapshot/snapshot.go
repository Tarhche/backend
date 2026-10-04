// Package snapshot is a VM's disk, kept as an archive.
//
// A snapshot is taken of a VM and outlives it: deleting a VM leaves its
// snapshots, and one can be restored onto the VM it came from, onto another
// of the same owner, kind and engine, or as a new VM.
package snapshot

import (
	"context"
	"io"
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

// ObjectKey is where a snapshot's archive is kept in the snapshots bucket.
func ObjectKey(uuid string) string {
	return "snapshots/" + uuid + ".msb"
}

// Repository stores snapshots.
type Repository interface {
	GetAll(ctx context.Context, offset uint, limit uint) ([]Snapshot, error)

	// GetAllByOwner is the same listing, of one person's own.
	GetAllByOwner(ctx context.Context, ownerUUID string, offset uint, limit uint) ([]Snapshot, error)

	// GetAllByVM is every snapshot taken of one VM.
	GetAllByVM(ctx context.Context, vmUUID string) ([]Snapshot, error)

	CountByOwner(ctx context.Context, ownerUUID string) (uint, error)
	Count(ctx context.Context) (uint, error)

	GetOne(ctx context.Context, uuid string) (Snapshot, error)

	// GetOneByOwner is one of somebody's own. A snapshot that is not theirs is
	// not there as far as they are concerned.
	GetOneByOwner(ctx context.Context, ownerUUID string, uuid string) (Snapshot, error)

	Save(ctx context.Context, s *Snapshot) (uuid string, err error)
	Delete(ctx context.Context, uuid string) error
}

// Store keeps the archives themselves, under their ObjectKey. An archive is
// streamed rather than held, so a size of -1 stores one whose length is not
// known until it ends.
type Store interface {
	Store(ctx context.Context, objectName string, reader io.Reader, objectSize int64) error
	Read(ctx context.Context, objectName string) (io.ReadSeekCloser, error)
	Delete(ctx context.Context, objectName string) error
}
