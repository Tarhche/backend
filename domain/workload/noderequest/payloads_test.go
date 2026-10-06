package noderequest

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/khanzadimahdi/testproject/domain/workload/docker"
	"github.com/khanzadimahdi/testproject/domain/workload/port"
)

// travel sends a value the way a request or a reply does, and reads it back.
func travel[T any](t *testing.T, value T) T {
	t.Helper()

	payload, err := json.Marshal(value)
	require.NoError(t, err)

	var arrived T
	require.NoError(t, json.Unmarshal(payload, &arrived))

	return arrived
}

var at = time.Date(2026, 10, 4, 12, 0, 0, 0, time.UTC)

func dockerFilter() docker.ContainerFilter {
	return docker.ContainerFilter{All: true, Stack: "shop-abcde"}
}

// Whatever leaves one end as a domain value arrives at the other as the same
// value.
func TestPayloads(t *testing.T) {
	t.Parallel()

	t.Run("a listing of containers", func(t *testing.T) {
		t.Parallel()

		assert.Equal(t, dockerFilter(), travel(t, NewContainersRequest(dockerFilter())).ToDocker())
	})

	t.Run("a container", func(t *testing.T) {
		t.Parallel()

		container := docker.Container{
			ID:            "c0ffee",
			Name:          "shop-abcde-web-1",
			Image:         "nginx:1.29",
			State:         "running",
			Status:        "Up 2 minutes",
			Command:       "nginx -g 'daemon off;'",
			Ports:         []docker.PortBinding{{ContainerPort: 80, HostPort: 8080, Protocol: "tcp"}},
			Networks:      []string{"shop-abcde_default"},
			Mounts:        []docker.Mount{{Type: "volume", Source: "data", Target: "/data", ReadOnly: true}},
			Labels:        map[string]string{docker.LabelComposeProject: "shop-abcde", docker.LabelComposeService: "web"},
			Stack:         "shop-abcde",
			Service:       "web",
			RestartPolicy: "unless-stopped",
			CreatedAt:     at,
		}

		assert.Equal(t, container, travel(t, NewContainer(container)).ToDocker())
	})

	t.Run("a container to create", func(t *testing.T) {
		t.Parallel()

		spec := docker.ContainerSpec{
			Name:          "web",
			Image:         "nginx:1.29",
			Command:       []string{"nginx", "-g", "daemon off;"},
			Entrypoint:    []string{"/docker-entrypoint.sh"},
			Env:           []string{"A=1"},
			WorkingDir:    "/srv",
			Ports:         []docker.PortBinding{{ContainerPort: 80, HostPort: 80, Protocol: "tcp"}},
			Mounts:        []docker.Mount{{Type: "bind", Source: "/srv", Target: "/srv"}},
			Networks:      []string{"backend"},
			RestartPolicy: "always",
			CPUs:          0.5,
			Memory:        256 << 20,
			Labels:        map[string]string{"team": "shop"},
		}

		assert.Equal(t, spec, travel(t, NewContainerSpec(spec)).ToDocker())
	})

	t.Run("a container's log", func(t *testing.T) {
		t.Parallel()

		options := docker.LogOptions{Since: at, Tail: 50}
		request := travel(t, NewContainerLogsRequest("c0ffee", options))

		assert.Equal(t, "c0ffee", request.ID)
		assert.Equal(t, options, request.Options())

		line := docker.LogLine{At: at, Stream: "stderr", Line: "oops"}
		assert.Equal(t, line, travel(t, NewLogLine(line)).ToDocker())
	})

	t.Run("a container's stats", func(t *testing.T) {
		t.Parallel()

		stats := docker.Stats{
			CPUPercent:  3.5,
			MemoryUsed:  64 << 20,
			MemoryLimit: 256 << 20,
			NetworkRx:   1,
			NetworkTx:   2,
			BlockRead:   3,
			BlockWrite:  4,
			PIDs:        5,
			SampledAt:   at,
		}

		assert.Equal(t, stats, travel(t, NewStats(stats)).ToDocker())
	})

	t.Run("images, networks and volumes", func(t *testing.T) {
		t.Parallel()

		image := docker.Image{ID: "sha256:1", Tags: []string{"nginx:1.29"}, Size: 1 << 20, CreatedAt: at, InUse: true}
		assert.Equal(t, image, travel(t, NewImage(image)).ToDocker())

		network := docker.Network{
			ID:         "n1",
			Name:       "backend",
			Driver:     "bridge",
			Scope:      "local",
			Internal:   true,
			Containers: []string{"c0ffee"},
			Labels:     map[string]string{"a": "b"},
			CreatedAt:  at,
		}
		assert.Equal(t, network, travel(t, NewNetwork(network)).ToDocker())

		networkSpec := docker.NetworkSpec{Name: "backend", Driver: "bridge", Internal: true, Labels: map[string]string{"a": "b"}}
		assert.Equal(t, networkSpec, travel(t, NewNetworkSpec(networkSpec)).ToDocker())

		volume := docker.Volume{Name: "data", Driver: "local", Mountpoint: "/var/lib/docker/volumes/data", Labels: map[string]string{"a": "b"}, InUse: true, CreatedAt: at}
		assert.Equal(t, volume, travel(t, NewVolume(volume)).ToDocker())

		volumeSpec := docker.VolumeSpec{Name: "data", Driver: "local", Labels: map[string]string{"a": "b"}}
		assert.Equal(t, volumeSpec, travel(t, NewVolumeSpec(volumeSpec)).ToDocker())
	})

	t.Run("nothing of a value is shared with what travels", func(t *testing.T) {
		t.Parallel()

		original := docker.Container{
			Ports:  []docker.PortBinding{{ContainerPort: 80}},
			Labels: map[string]string{"a": "b"},
		}

		travelling := NewContainer(original)
		travelling.Ports[0].ContainerPort = port.Port(8080)
		travelling.Labels["a"] = "changed"

		assert.Equal(t, port.Port(80), original.Ports[0].ContainerPort)
		assert.Equal(t, "b", original.Labels["a"])
	})
}
