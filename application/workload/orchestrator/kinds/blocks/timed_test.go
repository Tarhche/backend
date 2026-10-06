package blocks_test

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/khanzadimahdi/testproject/application/workload/orchestrator/kinds/blocks"
	"github.com/khanzadimahdi/testproject/application/workload/orchestrator/kinds/blocks/blockstest"
	"github.com/khanzadimahdi/testproject/domain"
	"github.com/khanzadimahdi/testproject/domain/workload/docker"
)

// timings are the requests to a dockerd a test's timings recorded.
type timings struct {
	lock sync.Mutex
	ops  []string
}

func (t *timings) DockerRequest(_ context.Context, op string, took time.Duration) {
	t.lock.Lock()
	defer t.lock.Unlock()

	if took >= 0 {
		t.ops = append(t.ops, op)
	}
}

func (t *timings) recorded() []string {
	t.lock.Lock()
	defer t.lock.Unlock()

	return append([]string(nil), t.ops...)
}

func TestTimed(t *testing.T) {
	t.Parallel()

	t.Run("every request to a dockerd is timed by its operation, answered or not", func(t *testing.T) {
		t.Parallel()

		node := blockstest.NewNode()
		node.DockerVM(t, "vm-1")

		took := &timings{}
		reader := blocks.NewReader(node.Engine, blocks.Timed(node, took), 0)

		_, err := reader.Read(t.Context())
		require.NoError(t, err)

		daemon, err := reader.Reach(t.Context(), "vm-1")
		require.NoError(t, err)

		_, err = daemon.PullImage(t.Context(), "redis:7")
		require.NoError(t, err)

		_, err = daemon.CreateContainer(t.Context(), docker.ContainerSpec{Name: "web", Image: "redis:7"})
		require.NoError(t, err)

		assert.ErrorIs(t, daemon.RemoveVolume(t.Context(), "nothing", false), domain.ErrNotExists)

		assert.Equal(t, []string{"docker.inventory", "docker.ping", "docker.images.pull", "docker.containers.create", "docker.volumes.remove"}, took.recorded())
	})

	t.Run("with nothing to record them into, the daemons are as they are", func(t *testing.T) {
		t.Parallel()

		node := blockstest.NewNode()

		assert.Same(t, blocks.Daemons(node), blocks.Timed(node, nil))
	})
}
