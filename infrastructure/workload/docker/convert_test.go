package docker

import (
	"testing"

	"github.com/docker/go-connections/nat"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/khanzadimahdi/testproject/domain/workload/docker"
)

func TestToCreate(t *testing.T) {
	t.Parallel()

	t.Run("what a container is asked for beyond the rest is what docker is asked for", func(t *testing.T) {
		t.Parallel()

		config, host, _ := toCreate(docker.ContainerSpec{
			Image: "redis:7",
			Ports: []docker.PortBinding{
				{ContainerPort: 6379, HostPort: 6379, HostIP: "127.0.0.1", Protocol: "tcp"},
				{ContainerPort: 6380, HostIP: "::1"},
			},
			PidsLimit:  100,
			Privileged: true,
			User:       "redis:redis",
			Runtime:    "runsc",
		})

		assert.Equal(t, "redis:redis", config.User)
		assert.True(t, host.Privileged)
		assert.Equal(t, "runsc", host.Runtime, "as it was given: one dockerd lacks is dockerd's to refuse")

		require.NotNil(t, host.PidsLimit)
		assert.Equal(t, int64(100), *host.PidsLimit)

		assert.Equal(t, []nat.PortBinding{{HostIP: "127.0.0.1", HostPort: "6379"}}, host.PortBindings["6379/tcp"])
		assert.Equal(t, []nat.PortBinding{{HostIP: "::1"}}, host.PortBindings["6380/tcp"], "on the address asked for, at a port docker picks")
	})

	t.Run("and what it is not asked for is left to docker", func(t *testing.T) {
		t.Parallel()

		config, host, networking := toCreate(docker.ContainerSpec{Image: "redis:7", Ports: []docker.PortBinding{{ContainerPort: 6379}}})

		assert.Empty(t, config.User, "its image's user")
		assert.False(t, host.Privileged)
		assert.Empty(t, host.Runtime, "dockerd's default runtime")
		assert.Nil(t, host.PidsLimit, "no limit of its own is dockerd's to say")
		assert.Equal(t, []nat.PortBinding{{}}, host.PortBindings["6379/tcp"], "on every address the vm has, at a port docker picks")
		assert.Nil(t, networking)
	})
}
