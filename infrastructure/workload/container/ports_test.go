package container

import (
	"testing"

	containerTypes "github.com/docker/docker/api/types/container"
	"github.com/docker/go-connections/nat"
	"github.com/stretchr/testify/assert"

	"github.com/khanzadimahdi/testproject/domain/workload/port"
)

func TestPublishAll(t *testing.T) {
	t.Parallel()

	t.Run("every exposed port is published on a host port docker picks", func(t *testing.T) {
		t.Parallel()

		published := convertPortMap(port.PortMap{
			80:  {{HostIP: "0.0.0.0"}},
			443: {{HostIP: "0.0.0.0"}},
		})

		assert.Equal(t, nat.PortMap{
			"80/tcp":  {{HostIP: "0.0.0.0"}},
			"443/tcp": {{HostIP: "0.0.0.0"}},
		}, published)
	})
}

func TestEndpoints(t *testing.T) {
	t.Parallel()

	t.Run("only a port docker published can be reached", func(t *testing.T) {
		t.Parallel()

		inspected := nat.PortMap{
			"80/tcp":   {{HostIP: "0.0.0.0", HostPort: "32768"}},
			"8080/tcp": {{HostIP: "0.0.0.0", HostPort: ""}},
			"443/tcp":  nil,
			"22/tcp":   {{HostIP: "0.0.0.0", HostPort: "32769"}},
		}

		// the lowest first, whatever order docker hands them back in.
		assert.Equal(t, []port.Port{22, 80}, inspectedEndpoints(inspected))
		assert.Equal(t, port.PortSet{80: {}, 8080: {}, 443: {}, 22: {}}, convertDockerPortSetFromMap(inspected))

		hostPort, found := publishedPort(inspected, 80)
		assert.True(t, found)
		assert.Equal(t, "32768", hostPort)

		_, found = publishedPort(inspected, 8080)
		assert.False(t, found)

		_, found = publishedPort(inspected, 9090)
		assert.False(t, found)
	})

	t.Run("a listing names a published port once per address family", func(t *testing.T) {
		t.Parallel()

		listed := []containerTypes.Port{
			{PrivatePort: 80, PublicPort: 32768, IP: "0.0.0.0"},
			{PrivatePort: 80, PublicPort: 32768, IP: "::"},
			{PrivatePort: 22, PublicPort: 32769, IP: "0.0.0.0"},
			{PrivatePort: 8080},
		}

		assert.Equal(t, []port.Port{22, 80}, listedEndpoints(listed))
		assert.Equal(t, port.PortSet{80: {}, 22: {}, 8080: {}}, convertDockerPortSet(listed))
	})

	t.Run("nothing published is nothing reachable", func(t *testing.T) {
		t.Parallel()

		assert.Empty(t, listedEndpoints(nil))
		assert.Empty(t, inspectedEndpoints(nil))
	})
}
