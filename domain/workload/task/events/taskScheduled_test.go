package events

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/khanzadimahdi/testproject/domain/workload/runtime"
	"github.com/khanzadimahdi/testproject/domain/workload/task"
)

func TestNewTaskScheduled_Runtime(t *testing.T) {
	t.Parallel()

	t.Run("a task is scheduled with the class it was created with", func(t *testing.T) {
		t.Parallel()

		scheduled := NewTaskScheduled(&task.Task{UUID: "t-1", Runtime: runtime.Firecracker}, "", "workload-orchestrator-01", 0)

		assert.Equal(t, runtime.Firecracker, scheduled.Runtime)
	})

	t.Run("a task from before there were classes is scheduled as sysbox", func(t *testing.T) {
		t.Parallel()

		scheduled := NewTaskScheduled(&task.Task{UUID: "t-1"}, "", "workload-orchestrator-01", 0)

		assert.Equal(t, runtime.Sysbox, scheduled.Runtime)
	})

	t.Run("a message from a control plane older than classes reads as naming none", func(t *testing.T) {
		t.Parallel()

		var scheduled TaskScheduled
		require.NoError(t, json.Unmarshal([]byte(`{"uuid":"t-1","image":"nginx"}`), &scheduled))

		assert.Equal(t, runtime.Sysbox, scheduled.Runtime.OrSysbox())
	})
}
