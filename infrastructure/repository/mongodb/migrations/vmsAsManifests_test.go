package migrations

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.mongodb.org/mongo-driver/v2/bson"
)

func TestVMStates(t *testing.T) {
	t.Parallel()

	for name, tt := range map[string]struct {
		state, expected int
		reason          string
		want, wanted    string
		says            string
	}{
		"a running vm runs":                                     {state: 4, expected: 4, want: "running", wanted: "running"},
		"a stopped one stays stopped":                           {state: 6, expected: 6, want: "stopped", wanted: "stopped"},
		"one stopped that is to run is started":                 {state: 6, expected: 4, want: "stopped", wanted: "running"},
		"a failed one is failed, and brought back":              {state: 9, expected: 4, reason: "out of memory", want: "failed", wanted: "running", says: "out of memory"},
		"one given up on stays given up on":                     {state: 9, expected: 9, reason: "no_capacity", want: "failed", wanted: "failed", says: "no_capacity"},
		"one that failed saying nothing says it failed":         {state: 9, expected: 4, want: "failed", wanted: "running", says: "the vm failed"},
		"one never made is made":                                {state: 1, expected: 4, want: "created", wanted: "running"},
		"and so is one scheduled, which its node may have made": {state: 2, expected: 4, want: "created", wanted: "running"},
		"one never made that is to stay stopped stays as it is": {state: 2, expected: 6, want: "created", wanted: "stopped"},
		"one being started is stopped, and to be running":       {state: 3, expected: 4, want: "stopped", wanted: "running"},
		"one being stopped runs, and is to be stopped":          {state: 5, expected: 6, want: "running", wanted: "stopped"},
		"one being restarted runs":                              {state: 7, expected: 4, want: "running", wanted: "running"},
		"one whose restore was cut short failed, saying so":     {state: 8, expected: 4, want: "failed", wanted: "running", says: "the restore was cut short"},
		"one being deleted failed, and is to be deleted":        {state: 10, expected: 10, want: "failed", wanted: "deleted"},
		"one that says nothing it was asked is to be running":   {state: 4, expected: 0, want: "running", wanted: "running"},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			state, expected, reason := vmStates(tt.state, tt.expected, tt.reason)

			assert.Equal(t, tt.want, state)
			assert.Equal(t, tt.wanted, expected)
			assert.Equal(t, tt.says, reason)
		})
	}
}

func TestVMManifest(t *testing.T) {
	t.Parallel()

	created := time.Date(2026, 10, 5, 12, 0, 0, 0, time.UTC)

	running := oldVM{
		UUID:           "vm-uuid",
		Name:           "box",
		Slug:           "box-abcde",
		OwnerUUID:      "owner-uuid",
		Kind:           "docker",
		Image:          "docker:29-dind",
		Resources:      oldResources{CPUs: 2, Memory: 2 << 30, Disk: 20 << 30},
		Ports:          []int64{22, 80},
		Network:        oldNetwork{Ingress: "allow", Egress: "deny"},
		PersistentDisk: true,
		Lifetime:       time.Hour,
		ExpiresAt:      created.Add(time.Hour),
		CurrentState:   4,
		ExpectedState:  4,
		NodeName:       "workload-orchestrator-01",
		Stats:          oldStats{CPUPercent: 12.5, MemoryUsed: 256 << 20, MemoryLimit: 2 << 30, SampledAt: created.Add(time.Minute)},
		RestoreFrom:    "",

		LastHeartbeatAt: created.Add(2 * time.Minute),
		CreatedAt:       created,
		StartedAt:       created.Add(time.Second),
		UpdatedAt:       created.Add(time.Second),
	}

	encoded, err := bson.MarshalExtJSON(vmManifest(running), false, false)
	require.NoError(t, err)

	assert.JSONEq(t, `{
		"_id": "vm-uuid",
		"kind": "vm",
		"metadata": {
			"name": "box",
			"slug": "box-abcde",
			"owner_uuid": "owner-uuid",
			"labels": {"workload.flavor": "docker"},
			"node": "workload-orchestrator-01",
			"lifetime": 3600000000000,
			"expires_at": {"$date": "2026-10-05T13:00:00Z"},
			"created_at": {"$date": "2026-10-05T12:00:00Z"},
			"updated_at": {"$date": "2026-10-05T12:00:01Z"}
		},
		"spec": {
			"flavor": "docker",
			"image": "docker:29-dind",
			"resources": {"cpus": 2, "memory": 2147483648, "disk": 21474836480},
			"ports": [22, 80],
			"network": {"ingress": "allow", "egress": "deny"},
			"persistent_disk": true
		},
		"status": {
			"state": "running",
			"expected": "running",
			"since": "2026-10-05T12:00:01Z",
			"observed_at": "2026-10-05T12:02:00Z",
			"stats": {"cpu_percent": 12.5, "memory_used": 268435456, "memory_limit": 2147483648, "disk_used": 0, "disk_total": 0, "network_rx": 0, "network_tx": 0, "sampled_at": "2026-10-05T12:01:00Z"},
			"endpoints": null,
			"started_at": "2026-10-05T12:00:01Z",
			"applied": {
				"resources": {"cpus": 2, "memory": 2147483648, "disk": 21474836480},
				"ports": [22, 80],
				"network": {"ingress": "allow", "egress": "deny"}
			}
		},
		"version": 1,
		"control": {}
	}`, string(encoded))

	t.Run("one never made is made from what it was to be made from, with nothing applied yet", func(t *testing.T) {
		t.Parallel()

		scheduled := running
		scheduled.CurrentState = 2
		scheduled.RestoreFrom = "snapshot-uuid"
		scheduled.Ports = nil
		scheduled.Lifetime = 0
		scheduled.ExpiresAt = time.Time{}
		scheduled.LastHeartbeatAt = time.Time{}
		scheduled.StartedAt = time.Time{}

		status, spec, metadata := parts(t, vmManifest(scheduled))

		assert.Equal(t, "created", status["state"])
		assert.NotContains(t, status, "applied")
		assert.NotContains(t, status, "observed_at")
		assert.NotContains(t, status, "started_at")
		assert.Nil(t, status["stats"], "it runs nowhere yet")
		assert.Equal(t, map[string]any{"snapshot": "snapshot-uuid"}, spec["source"])
		assert.Equal(t, []any{}, spec["ports"], "none rather than nothing")
		assert.NotContains(t, metadata, "lifetime", "kept until it is deleted")
		assert.NotContains(t, metadata, "expires_at")
	})

	t.Run("one whose restore was cut short is restored again by whoever asks, not made from it", func(t *testing.T) {
		t.Parallel()

		restoring := running
		restoring.CurrentState = 8
		restoring.RestoreFrom = "snapshot-uuid"

		status, spec, _ := parts(t, vmManifest(restoring))

		assert.Equal(t, "failed", status["state"])
		assert.Equal(t, "the restore was cut short", status["reason"])
		assert.NotContains(t, spec, "source")
		assert.Nil(t, status["stats"], "only a running one is sampled")
	})

	t.Run("one restarting is given its config again", func(t *testing.T) {
		t.Parallel()

		restarting := running
		restarting.CurrentState = 7

		status, spec, _ := parts(t, vmManifest(restarting))

		assert.Equal(t, "running", status["state"])
		require.Contains(t, status, "applied")
		assert.NotEqual(t, spec["ports"], status["applied"].(map[string]any)["ports"], "what was applied is not its spec, so it is reconfigured")
	})

	t.Run("one on no node is on none, and a machine says so", func(t *testing.T) {
		t.Parallel()

		nowhere := running
		nowhere.NodeName = ""
		nowhere.Kind = "machine"
		nowhere.Image = ""
		nowhere.PersistentDisk = false

		_, spec, metadata := parts(t, vmManifest(nowhere))

		assert.NotContains(t, metadata, "node")
		assert.Equal(t, map[string]any{"workload.flavor": "machine"}, metadata["labels"])
		assert.NotContains(t, spec, "image")
		assert.NotContains(t, spec, "persistent_disk")
	})
}

// parts are a converted VM's status, spec and metadata, as JSON reads them.
func parts(t *testing.T, converted bson.D) (map[string]any, map[string]any, map[string]any) {
	t.Helper()

	encoded, err := bson.MarshalExtJSON(converted, false, false)
	require.NoError(t, err)

	var manifest struct {
		Metadata map[string]any `json:"metadata"`
		Spec     map[string]any `json:"spec"`
		Status   map[string]any `json:"status"`
	}
	require.NoError(t, json.Unmarshal(encoded, &manifest))

	return manifest.Status, manifest.Spec, manifest.Metadata
}
