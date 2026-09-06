package cluster

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/khanzadimahdi/testproject/domain"
	"github.com/khanzadimahdi/testproject/domain/runner/task"
	"github.com/khanzadimahdi/testproject/domain/runner/task/events"
)

func beat(t *testing.T, heartbeat events.Heartbeat) []byte {
	t.Helper()

	payload, err := json.Marshal(heartbeat)
	require.NoError(t, err)

	return payload
}

func running(node string, slug string) events.Heartbeat {
	return events.Heartbeat{
		UUID:     "task-uuid",
		Name:     "task-name",
		Slug:     slug,
		NodeName: node,
		State:    int(task.Running),
		Endpoints: []events.Endpoint{{
			ContainerPort: 8080,
			Host:          node,
			HostPort:      32768,
			PublicPort:    30001,
			PublicHost:    "localhost",
		}},
	}
}

func TestView(t *testing.T) {
	t.Parallel()

	t.Run("finds a container another node is holding", func(t *testing.T) {
		t.Parallel()

		view := NewView("node-01")
		require.NoError(t, view.Handle(context.Background(), beat(t, running("node-02", "nginx-xkfqz"))))

		found, err := view.GetOneBySlug(context.Background(), "nginx-xkfqz")
		require.NoError(t, err)

		assert.Equal(t, "node-02", found.NodeName)
		require.Len(t, found.Endpoints, 1)
		assert.Equal(t, "node-02", found.Endpoints[0].Host, "which is where a request for it is passed on to")
	})

	t.Run("knows nothing of a container nobody has spoken for", func(t *testing.T) {
		t.Parallel()

		view := NewView("node-01")

		_, err := view.GetOneBySlug(context.Background(), "nginx-xkfqz")
		assert.ErrorIs(t, err, domain.ErrNotExists)
	})

	t.Run("forgets a container that has ended, without waiting for silence", func(t *testing.T) {
		t.Parallel()

		view := NewView("node-01")
		require.NoError(t, view.Handle(context.Background(), beat(t, running("node-01", "nginx-xkfqz"))))

		ended := running("node-01", "nginx-xkfqz")
		ended.State = int(task.Completed)
		require.NoError(t, view.Handle(context.Background(), beat(t, ended)))

		_, err := view.GetOneBySlug(context.Background(), "nginx-xkfqz")
		assert.ErrorIs(t, err, domain.ErrNotExists)
	})

	t.Run("forgets a node that has gone quiet", func(t *testing.T) {
		t.Parallel()

		view := NewView("node-01")

		now := time.Now()
		view.now = func() time.Time { return now }

		require.NoError(t, view.Handle(context.Background(), beat(t, running("node-02", "nginx-xkfqz"))))

		now = now.Add(staleAfter + time.Second)

		_, err := view.GetOneBySlug(context.Background(), "nginx-xkfqz")
		assert.ErrorIs(t, err, domain.ErrNotExists)

		view.Forget()

		mine, err := view.GetRunningWithPublicPorts(context.Background())
		require.NoError(t, err)
		assert.Empty(t, mine)
	})

	t.Run("listens on its own containers' ports and nobody else's", func(t *testing.T) {
		t.Parallel()

		view := NewView("node-01")
		require.NoError(t, view.Handle(context.Background(), beat(t, running("node-01", "mine-abcde"))))
		require.NoError(t, view.Handle(context.Background(), beat(t, running("node-02", "theirs-fghij"))))

		mine, err := view.GetRunningWithPublicPorts(context.Background())
		require.NoError(t, err)

		require.Len(t, mine, 1, "another node's ports are that node's to listen on")
		assert.Equal(t, "mine-abcde", mine[0].Slug)
	})

	t.Run("a beat nobody can read says nothing about anything", func(t *testing.T) {
		t.Parallel()

		view := NewView("node-01")
		assert.NoError(t, view.Handle(context.Background(), []byte("not json")))
	})
}
