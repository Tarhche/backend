// Package presenter is how the control plane's API shows a snapshot:
// snake_case JSON, states as words and sizes as bytes. A kind on the
// framework, a VM, a stack or a building block of a Docker VM, is shown as
// its manifest instead.
//
// The blog's control plane client reads exactly these shapes back, so a field
// renamed here is renamed there too.
package presenter

import (
	"time"

	"github.com/khanzadimahdi/testproject/domain/workload/snapshot"
)

// Pagination says where a page is in its listing.
type Pagination struct {
	TotalPages  uint `json:"total_pages"`
	CurrentPage uint `json:"current_page"`
}

// NewPagination is the page currentPage of a listing of total items, limit to
// a page.
func NewPagination(total uint, limit uint, currentPage uint) Pagination {
	totalPages := total / limit
	if totalPages*limit != total {
		totalPages++
	}

	return Pagination{TotalPages: totalPages, CurrentPage: currentPage}
}

// Offset is where page starts in a listing of limit to a page. Page zero is
// the first page, as page one is.
func Offset(page uint, limit uint) (uint, uint) {
	if page == 0 {
		page = 1
	}

	return (page - 1) * limit, page
}

// Snapshot is a snapshot as the API shows it.
type Snapshot struct {
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

func NewSnapshot(s *snapshot.Snapshot) Snapshot {
	return Snapshot{
		UUID:        s.UUID,
		Name:        s.Name,
		OwnerUUID:   s.OwnerUUID,
		VMUUID:      s.VMUUID,
		VMName:      s.VMName,
		Kind:        string(s.Kind),
		Image:       s.Image,
		Disk:        s.Disk,
		Engine:      s.Engine,
		Size:        s.Size,
		State:       s.State.String(),
		Reason:      s.Reason,
		CreatedAt:   s.CreatedAt,
		CompletedAt: s.CompletedAt,
	}
}

func NewSnapshots(snapshots []snapshot.Snapshot) []Snapshot {
	items := make([]Snapshot, len(snapshots))
	for i := range snapshots {
		items[i] = NewSnapshot(&snapshots[i])
	}

	return items
}
