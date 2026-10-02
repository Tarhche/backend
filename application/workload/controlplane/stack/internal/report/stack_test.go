package report

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/khanzadimahdi/testproject/domain/workload/runtime"
	"github.com/khanzadimahdi/testproject/domain/workload/stack"
	"github.com/khanzadimahdi/testproject/domain/workload/task"
)

func TestNewStack_Runtime(t *testing.T) {
	t.Parallel()

	t.Run("a stack says which class it runs with, and each service which node holds it", func(t *testing.T) {
		t.Parallel()

		reported := NewStack(
			stack.Stack{UUID: "stack-uuid", Runtime: runtime.Firecracker, NodeName: "workload-orchestrator-01"},
			[]task.Task{
				{UUID: "web-uuid", ServiceName: "web", Runtime: runtime.Firecracker, NodeName: "workload-orchestrator-01", CurrentState: task.Running},
				{UUID: "db-uuid", ServiceName: "db", Runtime: runtime.Firecracker, CurrentState: task.Created},
			},
		)

		encoded, err := json.Marshal(reported)
		require.NoError(t, err)

		var fields struct {
			Runtime  string `json:"runtime"`
			Services []struct {
				Runtime  string `json:"runtime"`
				NodeName string `json:"node_name"`
			} `json:"services"`
		}
		require.NoError(t, json.Unmarshal(encoded, &fields))

		assert.Equal(t, "firecracker", fields.Runtime)
		require.Len(t, fields.Services, 2)
		assert.Equal(t, "firecracker", fields.Services[0].Runtime)
		assert.Equal(t, "workload-orchestrator-01", fields.Services[0].NodeName)
		assert.Empty(t, fields.Services[1].NodeName, "one not placed yet is on no node")
	})

	t.Run("a stack from before there were classes ran under sysbox", func(t *testing.T) {
		t.Parallel()

		reported := NewStack(stack.Stack{UUID: "stack-uuid"}, []task.Task{{UUID: "web-uuid"}})

		assert.Equal(t, "sysbox", reported.Runtime)
		assert.Equal(t, "sysbox", reported.Services[0].Runtime)
	})
}
