package tasks

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.mongodb.org/mongo-driver/v2/bson"

	"github.com/khanzadimahdi/testproject/domain/workload/network"
	"github.com/khanzadimahdi/testproject/domain/workload/runtime"
	"github.com/khanzadimahdi/testproject/domain/workload/task"
)

func TestTaskBson_Runtime(t *testing.T) {
	t.Parallel()

	t.Run("a task's class is stored, and read back", func(t *testing.T) {
		t.Parallel()

		stored := toBson(&task.Task{UUID: "task-uuid", Runtime: runtime.Firecracker, NetworkPolicy: network.PolicyIsolated})

		encoded, err := bson.Marshal(stored)
		require.NoError(t, err)

		var raw bson.M
		require.NoError(t, bson.Unmarshal(encoded, &raw))
		assert.Equal(t, "firecracker", raw["runtime"])

		var back TaskBson
		require.NoError(t, bson.Unmarshal(encoded, &back))

		assert.Equal(t, runtime.Firecracker, toTask(&back).Runtime)
	})

	t.Run("a task stored before there were classes ran under sysbox", func(t *testing.T) {
		t.Parallel()

		encoded, err := bson.Marshal(bson.M{"_id": "task-uuid", "name": "web", "image": "nginx:alpine"})
		require.NoError(t, err)

		var stored TaskBson
		require.NoError(t, bson.Unmarshal(encoded, &stored))

		assert.Equal(t, runtime.Sysbox, toTask(&stored).Runtime)
	})

	t.Run("a task with no class says nothing about it, so nothing stored is overwritten", func(t *testing.T) {
		t.Parallel()

		encoded, err := bson.Marshal(toBson(&task.Task{UUID: "task-uuid"}))
		require.NoError(t, err)

		var raw bson.M
		require.NoError(t, bson.Unmarshal(encoded, &raw))
		assert.NotContains(t, raw, "runtime")
	})
}
