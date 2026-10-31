package blockstest_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/khanzadimahdi/testproject/application/workload/orchestrator/kinds/blocks/blockstest"
	"github.com/khanzadimahdi/testproject/domain/workload/docker"
)

// TestDockerd holds the dockerd the node strategies are tested against to
// refusing what docker refuses, and to doing nothing where docker does
// nothing.
func TestDockerd(t *testing.T) {
	t.Parallel()

	ctx := context.Background()

	published := []docker.PortBinding{{ContainerPort: 80, HostPort: 8080, Protocol: "tcp"}}

	t.Run("a container made stopped is made though a running one holds its host port, and is refused it as it starts, as docker refuses it", func(t *testing.T) {
		t.Parallel()

		d := blockstest.NewDockerd()

		_, err := d.CreateContainer(ctx, docker.ContainerSpec{Name: "first", Image: "nginx:1.27", Ports: published})
		require.NoError(t, err)

		_, err = d.CreateContainer(ctx, docker.ContainerSpec{Name: "second", Image: "nginx:1.27", Ports: published})
		require.ErrorIs(t, err, docker.ErrInvalid, "made running, it is refused the port at once")

		stopped, err := d.CreateContainer(ctx, docker.ContainerSpec{Name: "third", Image: "nginx:1.27", Ports: published, Stopped: true})
		require.NoError(t, err, "made stopped, it takes no port yet")

		assert.ErrorIs(t, d.StartContainer(ctx, stopped.ID), docker.ErrInvalid, "and is refused it as it starts")
		assert.ErrorIs(t, d.RestartContainer(ctx, stopped.ID), docker.ErrInvalid)

		require.NoError(t, d.StopContainer(ctx, "first"))
		require.NoError(t, d.StartContainer(ctx, stopped.ID), "once the one that held it stopped, it starts")

		assert.ErrorIs(t, d.StartContainer(ctx, "first"), docker.ErrInvalid, "and the other is refused it in turn")
		assert.NoError(t, d.RestartContainer(ctx, stopped.ID), "while it is restarted onto its own")
		assert.NoError(t, d.StartContainer(ctx, stopped.ID), "and started again as it runs")
	})

	t.Run("a running container put on a network it is on is refused, and one that is not running is left as it was, as docker does nothing for it", func(t *testing.T) {
		t.Parallel()

		d := blockstest.NewDockerd()
		d.HoldNetwork(docker.Network{ID: "n1", Name: "backend"})

		_, err := d.CreateContainer(ctx, docker.ContainerSpec{Name: "web", Image: "nginx:1.27", Networks: []string{"backend"}, Aliases: map[string][]string{"backend": {"api"}}})
		require.NoError(t, err)

		assert.ErrorIs(t, d.ConnectNetwork(ctx, "backend", "web", []string{"web"}), docker.ErrInvalid)

		require.NoError(t, d.StopContainer(ctx, "web"))

		require.NoError(t, d.ConnectNetwork(ctx, "n1", "web", []string{"web"}))
		assert.Equal(t, []string{"api"}, d.Aliases("web", "backend"), "under the aliases it was put on it with")

		held := d.Held()
		require.Len(t, held, 1)
		assert.Equal(t, []string{"backend"}, held[0].Networks)
	})
}
