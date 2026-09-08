package heartbeat

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/khanzadimahdi/testproject/domain/runner/task"
	"github.com/khanzadimahdi/testproject/domain/runner/task/events"
)

func TestDeadline(t *testing.T) {
	t.Parallel()

	ends := time.Now().Add(90 * time.Second)

	t.Run("a snippet somebody is watching says when it will be stopped", func(t *testing.T) {
		t.Parallel()

		at := deadline(&events.Heartbeat{
			Interactive: true,
			Deadline:    ends,
		}, task.Running)

		require.NotNil(t, at)
		assert.Equal(t, ends, *at)
	})

	t.Run("a snippet nobody is watching is answered once and counts down to nothing", func(t *testing.T) {
		t.Parallel()

		assert.Nil(t, deadline(&events.Heartbeat{Deadline: ends}, task.Running))
	})

	t.Run("a snippet that is no longer running has nothing left", func(t *testing.T) {
		t.Parallel()

		assert.Nil(t, deadline(&events.Heartbeat{
			Interactive: true,
			Deadline:    ends,
		}, task.Completed))
	})

	t.Run("a container that may run for as long as it likes has no deadline", func(t *testing.T) {
		t.Parallel()

		assert.Nil(t, deadline(&events.Heartbeat{Interactive: true}, task.Running))
	})
}

func TestEndpoints_node(t *testing.T) {
	t.Parallel()

	beat := events.Heartbeat{
		Slug:        "abc-xkfqz",
		Interactive: true,
		Endpoints: []events.Endpoint{
			{ContainerPort: 8080},
			{ContainerPort: 3000},
		},
	}

	t.Run("a snippet is reached on the node holding it", func(t *testing.T) {
		t.Parallel()

		beat := beat
		beat.IngressDomain = "node-02.runner.tarhche.com"

		handler := NewHeartbeatHandler(nil, "runner.tarhche.com", discardLogger())
		endpoints := handler.endpoints(&beat, task.Running)

		require.Len(t, endpoints, 2)
		assert.Equal(t, "http://abc-xkfqz.node-02.runner.tarhche.com", endpoints[0].URL, "the first port answers on the container's bare name")
		assert.Equal(t, "http://abc-xkfqz-3000.node-02.runner.tarhche.com", endpoints[1].URL, "and the rest carry their port in it")
	})

	t.Run("a node that names no domain of its own is answered under the runner's", func(t *testing.T) {
		t.Parallel()

		handler := NewHeartbeatHandler(nil, "runner.tarhche.com", discardLogger())
		endpoints := handler.endpoints(&beat, task.Running)

		require.Len(t, endpoints, 2)
		assert.Equal(t, "http://abc-xkfqz.runner.tarhche.com", endpoints[0].URL)
	})
}
