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
