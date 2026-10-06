package snapshot

import (
	"github.com/khanzadimahdi/testproject/domain/workload/kind"
	"github.com/khanzadimahdi/testproject/domain/workload/snapshot"
)

// StateOf is what the dashboard has always called the state a snapshot is
// in: a snapshot expected deleted, and one being deleted, are deleting,
// whatever they are doing until they are.
func StateOf(status kind.Status) snapshot.State {
	if status.Expected == Deleted {
		return snapshot.Deleting
	}

	switch status.State {
	case Creating:
		return snapshot.Creating
	case Ready:
		return snapshot.Ready
	case Failed:
		return snapshot.Failed
	case Deleting, Deleted:
		return snapshot.Deleting
	}

	return 0
}

// Entity is a snapshot's manifest as the snapshot package's Snapshot, which
// is what the dashboard shows a snapshot as. A snapshot that failed was done
// with when it failed.
func Entity(s Snapshot) snapshot.Snapshot {
	completed := s.Status.CompletedAt
	if completed.IsZero() && s.Status.State == Failed {
		completed = s.Status.Since
	}

	return snapshot.Snapshot{
		UUID:        s.Metadata.UUID,
		Name:        s.Metadata.Name,
		OwnerUUID:   s.Metadata.OwnerUUID,
		VMUUID:      VMOf(s),
		VMName:      s.Spec.VM.Name,
		Kind:        s.Status.Flavor,
		Image:       s.Status.Image,
		Disk:        s.Status.Disk,
		Engine:      s.Status.Engine,
		Size:        s.Status.Size,
		State:       StateOf(s.Status.Status),
		Reason:      s.Status.Reason,
		CreatedAt:   s.Metadata.CreatedAt,
		CompletedAt: completed,
	}
}
