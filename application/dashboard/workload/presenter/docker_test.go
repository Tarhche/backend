package presenter

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	workloadControlPlane "github.com/khanzadimahdi/testproject/domain/workload/controlplane"
	"github.com/khanzadimahdi/testproject/domain/workload/docker"
)

func web() docker.Container {
	return docker.Container{
		ID:            "c0ffee",
		Name:          "web",
		Image:         "nginx:1.27",
		State:         "running",
		Status:        "Up 3 minutes",
		Command:       "nginx -g 'daemon off;'",
		Ports:         []docker.PortBinding{{ContainerPort: 80, HostPort: 8080, Protocol: "tcp"}},
		Networks:      []string{"bridge", "backend"},
		Mounts:        []docker.Mount{{Type: "volume", Source: "data", Target: "/data", ReadOnly: true}},
		Labels:        map[string]string{"com.docker.compose.project": "shop-abcde"},
		Stack:         "shop-abcde",
		Service:       "web",
		RestartPolicy: "unless-stopped",
		CreatedAt:     at,
	}
}

func TestNewContainer(t *testing.T) {
	t.Parallel()

	t.Run("a container as dockerd reported it", func(t *testing.T) {
		t.Parallel()

		presented, err := json.Marshal(NewContainer(web()))
		require.NoError(t, err)

		assert.JSONEq(t, `{
			"id": "c0ffee",
			"name": "web",
			"image": "nginx:1.27",
			"state": "running",
			"status": "Up 3 minutes",
			"command": "nginx -g 'daemon off;'",
			"ports": [{"container_port": 80, "host_port": 8080, "protocol": "tcp"}],
			"networks": ["bridge", "backend"],
			"mounts": [{"type": "volume", "source": "data", "target": "/data", "read_only": true}],
			"labels": {"com.docker.compose.project": "shop-abcde"},
			"stack": "shop-abcde",
			"service": "web",
			"restart_policy": "unless-stopped",
			"created_at": "2026-10-04T12:00:00Z"
		}`, string(presented))
	})

	t.Run("one nobody keeps says so", func(t *testing.T) {
		t.Parallel()

		unmanaged := web()
		unmanaged.Unmanaged = true

		presented, err := json.Marshal(NewContainer(unmanaged))
		require.NoError(t, err)

		var fields map[string]json.RawMessage
		require.NoError(t, json.Unmarshal(presented, &fields))
		assert.JSONEq(t, `true`, string(fields["unmanaged"]))
	})

	t.Run("what a container has none of is an empty list, not null", func(t *testing.T) {
		t.Parallel()

		presented, err := json.Marshal(NewContainer(docker.Container{ID: "c0ffee", State: "exited", CreatedAt: at}))
		require.NoError(t, err)

		assert.JSONEq(t, `{
			"id": "c0ffee",
			"name": "",
			"image": "",
			"state": "exited",
			"status": "",
			"command": "",
			"ports": [],
			"networks": [],
			"mounts": [],
			"created_at": "2026-10-04T12:00:00Z"
		}`, string(presented))
	})
}

func TestNewVMContainers(t *testing.T) {
	t.Parallel()

	presented, err := json.Marshal(NewVMContainers([]workloadControlPlane.VMContainer{{
		Container: docker.Container{ID: "c0ffee", Name: "web", State: "running", CreatedAt: at},
		VMUUID:    "vm-uuid",
		VMName:    "docker-1",
	}}))
	require.NoError(t, err)

	// a listing across VMs says where each one was found, beside the
	// container itself rather than around it.
	assert.JSONEq(t, `[{
		"id": "c0ffee",
		"name": "web",
		"image": "",
		"state": "running",
		"status": "",
		"command": "",
		"ports": [],
		"networks": [],
		"mounts": [],
		"created_at": "2026-10-04T12:00:00Z",
		"vm_uuid": "vm-uuid",
		"vm_name": "docker-1"
	}]`, string(presented))
}

func TestDockerObjects(t *testing.T) {
	t.Parallel()

	t.Run("an image, its size in bytes", func(t *testing.T) {
		t.Parallel()

		presented, err := json.Marshal(NewImages([]docker.Image{
			{ID: "sha256:1", Tags: []string{"nginx:1.27"}, Size: 192 << 20, CreatedAt: at, InUse: true},
			{ID: "sha256:2", CreatedAt: at, Unmanaged: true},
		}))
		require.NoError(t, err)

		assert.JSONEq(t, `[
			{"id": "sha256:1", "tags": ["nginx:1.27"], "size": 201326592, "created_at": "2026-10-04T12:00:00Z", "in_use": true},
			{"id": "sha256:2", "tags": [], "size": 0, "created_at": "2026-10-04T12:00:00Z", "in_use": false, "unmanaged": true}
		]`, string(presented))
	})

	t.Run("a network, and who is on it", func(t *testing.T) {
		t.Parallel()

		presented, err := json.Marshal(NewDockerNetworks([]docker.Network{
			{ID: "n1", Name: "backend", Driver: "bridge", Scope: "local", Internal: true, Containers: []string{"web"}, Labels: map[string]string{"a": "b"}, CreatedAt: at, Unmanaged: true},
		}))
		require.NoError(t, err)

		assert.JSONEq(t, `[{
			"id": "n1", "name": "backend", "driver": "bridge", "scope": "local", "internal": true,
			"containers": ["web"], "labels": {"a": "b"}, "created_at": "2026-10-04T12:00:00Z", "unmanaged": true
		}]`, string(presented))
	})

	t.Run("a volume", func(t *testing.T) {
		t.Parallel()

		presented, err := json.Marshal(NewVolumes([]docker.Volume{
			{Name: "data", Driver: "local", Mountpoint: "/var/lib/docker/volumes/data/_data", InUse: true, CreatedAt: at},
			{Name: "scratch", Driver: "local", CreatedAt: at, Unmanaged: true},
		}))
		require.NoError(t, err)

		assert.JSONEq(t, `[{
			"name": "data", "driver": "local", "mountpoint": "/var/lib/docker/volumes/data/_data",
			"in_use": true, "created_at": "2026-10-04T12:00:00Z"
		}, {
			"name": "scratch", "driver": "local", "mountpoint": "",
			"in_use": false, "created_at": "2026-10-04T12:00:00Z", "unmanaged": true
		}]`, string(presented))
	})

	t.Run("a sample of what a container uses, in bytes", func(t *testing.T) {
		t.Parallel()

		presented, err := json.Marshal(NewContainerStats(docker.Stats{
			CPUPercent: 3.5, MemoryUsed: 64 << 20, MemoryLimit: 256 << 20,
			NetworkRx: 1, NetworkTx: 2, BlockRead: 3, BlockWrite: 4, PIDs: 5, SampledAt: at,
		}))
		require.NoError(t, err)

		assert.JSONEq(t, `{
			"cpu_percent": 3.5, "memory_used": 67108864, "memory_limit": 268435456,
			"network_rx": 1, "network_tx": 2, "block_read": 3, "block_write": 4, "pids": 5,
			"sampled_at": "2026-10-04T12:00:00Z"
		}`, string(presented))
	})

	t.Run("a container's log, line by line", func(t *testing.T) {
		t.Parallel()

		presented, err := json.Marshal(NewLogLines([]docker.LogLine{{At: at, Stream: "stderr", Line: "listening"}}))
		require.NoError(t, err)

		assert.JSONEq(t, `[{"at": "2026-10-04T12:00:00Z", "stream": "stderr", "line": "listening"}]`, string(presented))
	})
}
