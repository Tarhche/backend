package events

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/khanzadimahdi/testproject/domain/workload/node"
	"github.com/khanzadimahdi/testproject/domain/workload/vm"
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

	t.Run("and one is sent as it always was, saying nothing of what the node holds", func(t *testing.T) {
		t.Parallel()

		payload, err := json.Marshal(Heartbeat{Name: "workload-orchestrator-01", Role: node.OrchestratorRole, At: at})
		require.NoError(t, err)

		var fields map[string]json.RawMessage
		require.NoError(t, json.Unmarshal(payload, &fields))

		assert.ElementsMatch(t, []string{"Name", "Role", "Stats", "At"}, keys(fields))
	})

	t.Run("what a node offers arrives as it left", func(t *testing.T) {
		t.Parallel()

		sent := Heartbeat{
			Name:     "workload-orchestrator-01",
			Role:     node.OrchestratorRole,
			At:       at,
			Capacity: vm.Info{Engine: "microsandbox", Version: "0.7.6", CPUs: 16, Memory: 64 << 30, Disk: 1 << 40, Allocated: vm.Resources{CPUs: 2, Memory: 2 << 30, Disk: 10 << 30}},
		}

		payload, err := json.Marshal(sent)
		require.NoError(t, err)

		var arrived Heartbeat
		require.NoError(t, json.Unmarshal(payload, &arrived))

		assert.Equal(t, sent, arrived)
	})

	t.Run("one from a node that still says what its kinds hold in it is read for what it says of the node", func(t *testing.T) {
		t.Parallel()

		var heartbeat Heartbeat
		require.NoError(t, json.Unmarshal([]byte(`{"Name":"workload-orchestrator-01","Role":"orchestrator","At":"2026-10-06T12:00:00Z","observations":{"vm":{"instances":[]}}}`), &heartbeat))

		assert.Equal(t, Heartbeat{Name: "workload-orchestrator-01", Role: node.OrchestratorRole, At: at}, heartbeat)
	})
}

func keys(fields map[string]json.RawMessage) []string {
	var named []string
	for name := range fields {
		named = append(named, name)
	}

	return named
}
