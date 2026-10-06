package client

import (
	"time"

	workloadControlPlane "github.com/khanzadimahdi/testproject/domain/workload/controlplane"
	"github.com/khanzadimahdi/testproject/domain/workload/snapshot"
	"github.com/khanzadimahdi/testproject/domain/workload/vm"
)

// the shapes the control plane's API speaks about snapshots, which are not a
// kind yet; a VM, a stack and the building blocks of a Docker VM are
// manifests.

type paginationPayload struct {
	TotalPages  uint `json:"total_pages"`
	CurrentPage uint `json:"current_page"`
}

type snapshotPayload struct {
	UUID        string    `json:"uuid"`
	Name        string    `json:"name"`
	OwnerUUID   string    `json:"owner_uuid"`
	VMUUID      string    `json:"vm_uuid"`
	VMName      string    `json:"vm_name"`
	Kind        string    `json:"kind"`
	Image       string    `json:"image"`
	Disk        uint64    `json:"disk"`
	Engine      string    `json:"engine"`
	Size        int64     `json:"size"`
	State       string    `json:"state"`
	Reason      string    `json:"reason"`
	CreatedAt   time.Time `json:"created_at"`
	CompletedAt time.Time `json:"completed_at"`
}

// snapshotStates maps the words the API uses back onto a snapshot's own
// states.
var snapshotStates = map[string]snapshot.State{
	snapshot.Creating.String(): snapshot.Creating,
	snapshot.Ready.String():    snapshot.Ready,
	snapshot.Failed.String():   snapshot.Failed,
	snapshot.Deleting.String(): snapshot.Deleting,
}

func (p *snapshotPayload) toSnapshot() snapshot.Snapshot {
	return snapshot.Snapshot{
		UUID:        p.UUID,
		Name:        p.Name,
		OwnerUUID:   p.OwnerUUID,
		VMUUID:      p.VMUUID,
		VMName:      p.VMName,
		Kind:        vm.Kind(p.Kind),
		Image:       p.Image,
		Disk:        p.Disk,
		Engine:      p.Engine,
		Size:        p.Size,
		State:       snapshotStates[p.State],
		Reason:      p.Reason,
		CreatedAt:   p.CreatedAt,
		CompletedAt: p.CompletedAt,
	}
}

type snapshotPagePayload struct {
	Items      []snapshotPayload `json:"items"`
	Pagination paginationPayload `json:"pagination"`
}

func (p *snapshotPagePayload) toPage() workloadControlPlane.Page[snapshot.Snapshot] {
	items := make([]snapshot.Snapshot, len(p.Items))
	for i := range p.Items {
		items[i] = p.Items[i].toSnapshot()
	}

	return workloadControlPlane.Page[snapshot.Snapshot]{Items: items, TotalPages: p.Pagination.TotalPages, CurrentPage: p.Pagination.CurrentPage}
}
