package migrations

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"go.mongodb.org/mongo-driver/v2/bson"
)

func TestStackStates(t *testing.T) {
	t.Parallel()

	for name, tt := range map[string]struct {
		state, expected int
		want, wanted    string
	}{
		"a running stack runs":                                {state: 2, expected: 2, want: "running", wanted: "running"},
		"a stopped one stays stopped":                         {state: 5, expected: 5, want: "stopped", wanted: "stopped"},
		"a failed one is failed, and brought back":            {state: 8, expected: 2, want: "failed", wanted: "running"},
		"one being deployed waits to be":                      {state: 1, expected: 2, want: "waiting", wanted: "running"},
		"one being started is stopped, and to be running":     {state: 3, expected: 2, want: "stopped", wanted: "running"},
		"one being stopped runs, and is to be stopped":        {state: 4, expected: 5, want: "running", wanted: "stopped"},
		"one being restarted runs":                            {state: 6, expected: 2, want: "running", wanted: "running"},
		"one being taken down failed, and is to be deleted":   {state: 7, expected: 2, want: "failed", wanted: "deleted"},
		"one that says nothing it was asked is to be running": {state: 2, expected: 0, want: "running", wanted: "running"},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			state, expected := stackStates(tt.state, tt.expected)

			assert.Equal(t, tt.want, state)
			assert.Equal(t, tt.wanted, expected)
		})
	}
}

func TestStackManifest(t *testing.T) {
	t.Parallel()

	created := time.Date(2026, 10, 5, 12, 0, 0, 0, time.UTC)
	updated := created.Add(time.Hour)

	converted := stackManifest(oldStack{
		UUID:          "stack-uuid",
		Name:          "shop",
		OwnerUUID:     "owner-uuid",
		VMUUID:        "vm-uuid",
		Slug:          "shop-abcde",
		Compose:       "services: {web: {image: nginx}}",
		ExpectedState: 2,
		State:         1,
		Reason:        "waiting_for_vm",
		Output:        "",
		CreatedAt:     created,
		UpdatedAt:     updated,
	}, "workload-orchestrator-01")

	encoded, err := bson.MarshalExtJSON(converted, false, false)
	assert.NoError(t, err)

	assert.JSONEq(t, `{
		"_id": "stack-uuid",
		"kind": "stack",
		"metadata": {
			"name": "shop",
			"slug": "shop-abcde",
			"owner_uuid": "owner-uuid",
			"owners": [{"kind": "vm", "uuid": "vm-uuid"}],
			"node": "workload-orchestrator-01",
			"created_at": {"$date": "2026-10-05T12:00:00Z"},
			"updated_at": {"$date": "2026-10-05T13:00:00Z"}
		},
		"spec": {"vm": {"uuid": "vm-uuid"}, "compose": "services: {web: {image: nginx}}"},
		"status": {"state": "waiting", "expected": "running", "reason": "its vm is not running yet", "since": "2026-10-05T13:00:00Z"},
		"version": 1,
		"control": {}
	}`, string(encoded))

	gone, err := bson.MarshalExtJSON(stackManifest(oldStack{UUID: "stack-uuid", VMUUID: "gone", State: 2, ExpectedState: 2}, ""), false, false)
	assert.NoError(t, err)
	assert.NotContains(t, string(gone), `"node"`, "a stack whose vm is gone is on no node")
}
