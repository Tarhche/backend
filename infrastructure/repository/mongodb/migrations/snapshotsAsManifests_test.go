package migrations

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"go.mongodb.org/mongo-driver/v2/bson"
)

func TestSnapshotStates(t *testing.T) {
	t.Parallel()

	for name, tt := range map[string]struct {
		state                    int
		reason                   string
		want, wanted, wantReason string
	}{
		"a stored snapshot is ready": {state: 2, want: "ready", wanted: "ready"},
		"a failed one stays failed, saying why": {
			state: 3, reason: "no space left on device",
			want: "failed", wanted: "ready", wantReason: "no space left on device",
		},
		"and says something when it did not say": {
			state: 3,
			want:  "failed", wanted: "ready", wantReason: "the snapshot could not be taken",
		},
		"one being taken will never hear how it went": {
			state: 1,
			want:  "failed", wanted: "ready", wantReason: upgradedWhileTaken,
		},
		"one deleted while it was taken is still to be deleted": {
			state: 4,
			want:  "failed", wanted: "deleted", wantReason: upgradedWhileTaken,
		},
		"one that says nothing it is doing failed": {
			state: 0,
			want:  "failed", wanted: "ready", wantReason: "the snapshot could not be taken",
		},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			state, expected, reason := snapshotStates(tt.state, tt.reason)

			assert.Equal(t, tt.want, state)
			assert.Equal(t, tt.wanted, expected)
			assert.Equal(t, tt.wantReason, reason)
		})
	}
}

func TestSnapshotManifest(t *testing.T) {
	t.Parallel()

	created := time.Date(2026, 10, 5, 12, 0, 0, 0, time.UTC)

	converted := snapshotManifest(oldSnapshot{
		UUID:        "snapshot-uuid",
		Name:        "before the upgrade",
		OwnerUUID:   "owner-uuid",
		VMUUID:      "vm-uuid",
		VMName:      "box",
		Kind:        "machine",
		Image:       "ubuntu:24.04",
		Disk:        10 << 30,
		Engine:      "microsandbox/0.7.6",
		Size:        1 << 30,
		State:       2,
		CreatedAt:   created,
		CompletedAt: created.Add(time.Minute),
	})

	encoded, err := bson.MarshalExtJSON(converted, false, false)
	assert.NoError(t, err)

	assert.JSONEq(t, `{
		"_id": "snapshot-uuid",
		"kind": "snapshot",
		"metadata": {
			"name": "before the upgrade",
			"owner_uuid": "owner-uuid",
			"owners": [{"kind": "vm", "uuid": "vm-uuid"}],
			"created_at": {"$date": "2026-10-05T12:00:00Z"},
			"updated_at": {"$date": "2026-10-05T12:01:00Z"}
		},
		"spec": {"vm": {"uuid": "vm-uuid", "name": "box"}},
		"status": {
			"state": "ready",
			"expected": "ready",
			"since": "2026-10-05T12:01:00Z",
			"flavor": "machine",
			"image": "ubuntu:24.04",
			"disk": 10737418240,
			"engine": "microsandbox/0.7.6",
			"size": 1073741824,
			"completed_at": "2026-10-05T12:01:00Z"
		},
		"version": 1,
		"control": {}
	}`, string(encoded))

	t.Run("one being taken failed, since it was asked for, and was never done with", func(t *testing.T) {
		t.Parallel()

		taking, err := bson.MarshalExtJSON(snapshotManifest(oldSnapshot{UUID: "snapshot-uuid", VMUUID: "vm-uuid", Kind: "docker", State: 1, CreatedAt: created}), false, false)
		assert.NoError(t, err)

		assert.JSONEq(t, `{
			"_id": "snapshot-uuid",
			"kind": "snapshot",
			"metadata": {
				"name": "",
				"owner_uuid": "",
				"owners": [{"kind": "vm", "uuid": "vm-uuid"}],
				"created_at": {"$date": "2026-10-05T12:00:00Z"},
				"updated_at": {"$date": "2026-10-05T12:00:00Z"}
			},
			"spec": {"vm": {"uuid": "vm-uuid"}},
			"status": {
				"state": "failed",
				"expected": "ready",
				"reason": "the workload was upgraded while it was being taken",
				"since": "2026-10-05T12:00:00Z",
				"flavor": "docker"
			},
			"version": 1,
			"control": {}
		}`, string(taking))
	})
}
