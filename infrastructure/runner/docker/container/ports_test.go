package container

import (
	"slices"
	"testing"

	"github.com/docker/docker/api/types"
	"github.com/docker/go-connections/nat"
	"github.com/stretchr/testify/assert"

	"github.com/khanzadimahdi/testproject/domain/runner/network"
	"github.com/khanzadimahdi/testproject/domain/runner/port"
)

func TestPublishAll(t *testing.T) {
	t.Run("every exposed port is published on a host port docker picks", func(t *testing.T) {
		published := publishAll(port.PortSet{80: {}, 443: {}})

		assert.Equal(t, nat.PortMap{
			"80/tcp":  {{HostIP: "0.0.0.0"}},
			"443/tcp": {{HostIP: "0.0.0.0"}},
		}, published)
	})
}

func TestEndpoints(t *testing.T) {
	t.Run("only a port docker published can be reached", func(t *testing.T) {
		inspected := nat.PortMap{
			"80/tcp":   {{HostIP: "0.0.0.0", HostPort: "32768"}},
			"8080/tcp": {{HostIP: "0.0.0.0", HostPort: ""}},
			"443/tcp":  nil,
		}

		endpoints := inspectedEndpoints(inspected)

		assert.Equal(t, []port.Port{80}, endpoints)
		assert.Equal(t, port.PortSet{80: {}, 8080: {}, 443: {}}, inspectedExposedPorts(inspected))

		hostPort, found := publishedPort(inspected, 80)
		assert.True(t, found)
		assert.Equal(t, "32768", hostPort)

		_, found = publishedPort(inspected, 8080)
		assert.False(t, found)
	})

	t.Run("a listing names a published port once per address family", func(t *testing.T) {
		listed := []types.Port{
			{PrivatePort: 80, PublicPort: 32768, IP: "0.0.0.0"},
			{PrivatePort: 80, PublicPort: 32768, IP: "::"},
			{PrivatePort: 8080},
		}

		endpoints := listedEndpoints(listed)
		slices.Sort(endpoints)

		assert.Equal(t, []port.Port{80}, endpoints)
		assert.Equal(t, port.PortSet{80: {}, 8080: {}}, listedExposedPorts(listed))
	})
}

func TestNetworkNames(t *testing.T) {
	t.Run("the network that routes out is docker's own bridge", func(t *testing.T) {
		assert.Equal(t, "bridge", dockerNetwork(network.PublicNetworkName))
		assert.Equal(t, network.PublicNetworkName, runnerNetwork("bridge"))
	})

	t.Run("the runner's own networks are called the same thing in docker", func(t *testing.T) {
		assert.Equal(t, network.IsolatedNetworkName, dockerNetwork(network.IsolatedNetworkName))
		assert.Equal(t, network.StackNetworkName("shop"), runnerNetwork(network.StackNetworkName("shop")))
	})
}
