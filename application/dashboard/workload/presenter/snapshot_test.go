package presenter

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/khanzadimahdi/testproject/domain/workload/snapshot"
	"github.com/khanzadimahdi/testproject/domain/workload/vm"
)

func TestNewSnapshot(t *testing.T) {
	t.Parallel()

	t.Run("a snapshot that is ready, its sizes in bytes", func(t *testing.T) {
		t.Parallel()

		presented, err := json.Marshal(NewSnapshots([]snapshot.Snapshot{{
			UUID:        "snapshot-uuid",
			Name:        "before the upgrade",
			OwnerUUID:   "owner-uuid",
			VMUUID:      "vm-uuid",
			VMName:      "web",
			Kind:        vm.KindMachine,
			Image:       "ubuntu:24.04",
			Disk:        10 << 30,
			Engine:      "microsandbox/0.7.2",
			Size:        734003200,
			State:       snapshot.Ready,
			CreatedAt:   at,
			CompletedAt: at.Add(42e9),
		}}, NewOwners(nil)))
		require.NoError(t, err)

		assert.JSONEq(t, `[{
			"uuid": "snapshot-uuid",
			"name": "before the upgrade",
			"owner_uuid": "owner-uuid",
			"vm_uuid": "vm-uuid",
			"vm_name": "web",
			"kind": "machine",
			"image": "ubuntu:24.04",
			"disk": 10737418240,
			"engine": "microsandbox/0.7.2",
			"size": 734003200,
			"state": "ready",
			"created_at": "2026-10-04T12:00:00Z",
			"completed_at": "2026-10-04T12:00:42Z"
		}]`, string(presented))
	})

	t.Run("one that failed says why, and was never completed", func(t *testing.T) {
		t.Parallel()

		presented := NewSnapshot(snapshot.Snapshot{
			UUID:      "snapshot-uuid",
			State:     snapshot.Failed,
			Reason:    "the node went away",
			CreatedAt: at,
		}, NewOwners(nil))

		assert.Equal(t, "failed", presented.State)
		assert.Equal(t, "the node went away", presented.Reason)
		assert.Nil(t, presented.CompletedAt)
	})
}
