package events

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/khanzadimahdi/testproject/domain/workload/kind"
	"github.com/khanzadimahdi/testproject/domain/workload/node"
)

func TestHeartbeat(t *testing.T) {
	t.Parallel()

	at := time.Date(2026, 10, 6, 12, 0, 0, 0, time.UTC)

	t.Run("a heartbeat sent before kinds were is read as it always was", func(t *testing.T) {
		t.Parallel()

		var heartbeat Heartbeat
		require.NoError(t, json.Unmarshal([]byte(`{"Name":"workload-orchestrator-01","Role":"orchestrator","Stats":{"PIDs":7},"At":"2026-10-06T12:00:00Z"}`), &heartbeat))

		assert.Equal(t, Heartbeat{Name: "workload-orchestrator-01", Role: node.OrchestratorRole, Stats: node.Stats{PIDs: 7}, At: at}, heartbeat)
	})

	t.Run("and one from a node that runs no kinds is sent as it always was", func(t *testing.T) {
		t.Parallel()

		payload, err := json.Marshal(Heartbeat{Name: "workload-orchestrator-01", Role: node.OrchestratorRole, At: at})
		require.NoError(t, err)

		var fields map[string]json.RawMessage
		require.NoError(t, json.Unmarshal(payload, &fields))

		assert.ElementsMatch(t, []string{"Name", "Role", "Stats", "At"}, keys(fields))
	})

	t.Run("what every kind observed arrives as it left, by kind", func(t *testing.T) {
		t.Parallel()

		sent := Heartbeat{
			Name: "workload-orchestrator-01",
			Role: node.OrchestratorRole,
			At:   at,
			Observations: map[string]kind.Report[json.RawMessage]{
				"stack": {
					Instances: []kind.Observation{{
						Kind:   "stack",
						UUID:   "stack-uuid",
						Owners: []kind.Reference{{Kind: "vm", UUID: "vm-uuid"}},
						Status: json.RawMessage(`{"state":"degraded","reason":"1 of 2 services is not running"}`),
					}},
					Unseen: []string{"another-vm-uuid"},
				},
				"vm": {Instances: []kind.Observation{}},
			},
		}

		payload, err := json.Marshal(sent)
		require.NoError(t, err)

		var arrived Heartbeat
		require.NoError(t, json.Unmarshal(payload, &arrived))

		assert.Equal(t, sent, arrived)

		var fields map[string]json.RawMessage
		require.NoError(t, json.Unmarshal(payload, &fields))
		assert.Contains(t, fields, "observations")
	})
}

func keys(fields map[string]json.RawMessage) []string {
	var named []string
	for name := range fields {
		named = append(named, name)
	}

	return named
}
