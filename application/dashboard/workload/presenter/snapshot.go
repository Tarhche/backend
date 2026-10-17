package presenter

import (
	"time"

	"github.com/khanzadimahdi/testproject/domain/workload/snapshot"
)

// Snapshot is one snapshot of a VM's disk, as the dashboard shows it.
type Snapshot struct {
	UUID string `json:"uuid"`
	Name string `json:"name"`

	OwnerUUID string `json:"owner_uuid"`
	Owner     *Owner `json:"owner,omitempty"`

	// VMUUID and VMName are the VM it was taken of, which may be gone since.
	VMUUID string `json:"vm_uuid"`
	VMName string `json:"vm_name"`

	Kind  string `json:"kind"`
	Image string `json:"image"`

	// Disk is the disk, in bytes, a VM it is restored onto needs at least.
	Disk uint64 `json:"disk"`

	// Engine is what took it; only the same engine can restore it.
	Engine string `json:"engine,omitempty"`

	// Size is how many bytes it takes where it is kept.
	Size int64 `json:"size"`

	State  string `json:"state"`
	Reason string `json:"reason,omitempty"`

	CreatedAt   time.Time  `json:"created_at"`
	CompletedAt *time.Time `json:"completed_at,omitempty"`
}

func NewSnapshot(s snapshot.Snapshot, owners Owners) Snapshot {
	state := ""
	if s.State != 0 {
		state = s.State.String()
	}

	return Snapshot{
		UUID:        s.UUID,
		Name:        s.Name,
		OwnerUUID:   s.OwnerUUID,
		Owner:       owners.Of(s.OwnerUUID),
		VMUUID:      s.VMUUID,
		VMName:      s.VMName,
		Kind:        s.Kind.String(),
		Image:       s.Image,
		Disk:        s.Disk,
		Engine:      s.Engine,
		Size:        s.Size,
		State:       state,
		Reason:      s.Reason,
		CreatedAt:   s.CreatedAt,
		CompletedAt: when(s.CompletedAt),
	}
}

func NewSnapshots(snapshots []snapshot.Snapshot, owners Owners) []Snapshot {
	items := make([]Snapshot, len(snapshots))
	for i := range snapshots {
		items[i] = NewSnapshot(snapshots[i], owners)
	}

	return items
}
