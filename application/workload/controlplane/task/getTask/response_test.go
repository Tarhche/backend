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

func TestNewEndpoints(t *testing.T) {
	t.Parallel()

	t.Run("a port docker published is reachable", func(t *testing.T) {
		t.Parallel()

		assert.Equal(t, []EndpointResponse{{TaskPort: 80}}, NewEndpoints(task.Task{
			Endpoints: []task.Endpoint{{TaskPort: 80, HostPort: 32768}},
		}))
	})

	t.Run("so is one the node dials itself, which has no host port", func(t *testing.T) {
		t.Parallel()

		// a microVM publishes nothing on its host: the node holding it
		// reaches its ports through vmhost, and says which it can.
		assert.Equal(t, []EndpointResponse{{TaskPort: 80}, {TaskPort: 443}}, NewEndpoints(task.Task{
			Runtime:   runtime.Firecracker,
			Endpoints: []task.Endpoint{{TaskPort: 80}, {TaskPort: 443}},
		}))
	})

	t.Run("nothing reachable is an empty list", func(t *testing.T) {
		t.Parallel()

		endpoints := NewEndpoints(task.Task{})

		assert.NotNil(t, endpoints)
		assert.Empty(t, endpoints)
	})
}
