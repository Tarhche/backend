package gettask

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/khanzadimahdi/testproject/domain/workload/runtime"
	"github.com/khanzadimahdi/testproject/domain/workload/task"
)

func TestNewResponse_Runtime(t *testing.T) {
	t.Parallel()

	t.Run("a task says which class it runs with", func(t *testing.T) {
		t.Parallel()

		assert.Equal(t, "firecracker", NewResponse(task.Task{UUID: "task-uuid", Runtime: runtime.Firecracker}).Runtime)
	})

	t.Run("a task from before there were classes ran under sysbox", func(t *testing.T) {
		t.Parallel()

		assert.Equal(t, "sysbox", NewResponse(task.Task{UUID: "task-uuid"}).Runtime)
	})
}
