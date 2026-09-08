package presenter

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/khanzadimahdi/testproject/domain/runner/port"
	"github.com/khanzadimahdi/testproject/domain/runner/task"
	"github.com/khanzadimahdi/testproject/domain/user"
)

const ingressDomain = "runner.localhost:8021"

func TestNewEndpoints(t *testing.T) {
	t.Parallel()

	t.Run("the first port answers on the container's bare name", func(t *testing.T) {
		t.Parallel()

		endpoints := NewEndpoints(task.Task{
			Slug:      "nginx-xkfqz",
			Endpoints: []task.Endpoint{{ContainerPort: 80, Host: "docker", HostPort: 32768}},
		}, ingressDomain)

		require.Len(t, endpoints, 1)
		assert.Equal(t, uint(80), endpoints[0].ContainerPort)
		assert.Equal(t, "nginx-xkfqz.runner.localhost:8021", endpoints[0].Host)
		assert.Equal(t, "http://nginx-xkfqz.runner.localhost:8021", endpoints[0].URL)
	})

	t.Run("the rest carry the port in the name, which keeps it one label", func(t *testing.T) {
		t.Parallel()

		endpoints := NewEndpoints(task.Task{
			Slug: "app-xkfqz",
			Endpoints: []task.Endpoint{
				{ContainerPort: 80, Host: "docker", HostPort: 32768},
				{ContainerPort: 8080, Host: "docker", HostPort: 32769},
				{ContainerPort: 443, Host: "docker", HostPort: 32770},
			},
		}, ingressDomain)

		require.Len(t, endpoints, 3)
		assert.Equal(t, "app-xkfqz.runner.localhost:8021", endpoints[0].Host)
		assert.Equal(t, "app-xkfqz-8080.runner.localhost:8021", endpoints[1].Host)
		assert.Equal(t, "app-xkfqz-443.runner.localhost:8021", endpoints[2].Host)

		// one label under the domain either way, which is what a single
		// wildcard certificate can cover.
		for _, endpoint := range endpoints {
			label, rest, found := strings.Cut(endpoint.Host, ".")
			require.True(t, found)
			assert.NotContains(t, label, ".")
			assert.Equal(t, ingressDomain, rest)
		}
	})

	t.Run("an endpoint the runner reports is one it published", func(t *testing.T) {
		t.Parallel()

		// the runner keeps the node and host port to itself and lists only the
		// ports it actually published, so there is nothing here to second-guess
		// — dropping endpoints that arrive without a host port would drop all
		// of them.
		endpoints := NewEndpoints(task.Task{
			Slug:      "nginx-xkfqz",
			Endpoints: []task.Endpoint{{ContainerPort: 80}},
		}, ingressDomain)

		require.Len(t, endpoints, 1)
		assert.Equal(t, "http://nginx-xkfqz.runner.localhost:8021", endpoints[0].URL)
	})

	t.Run("a container with no name yet has no address to give", func(t *testing.T) {
		t.Parallel()

		assert.Empty(t, NewEndpoints(task.Task{
			Endpoints: []task.Endpoint{{ContainerPort: 80, HostPort: 32768}},
		}, ingressDomain))
	})

	t.Run("a container exposing nothing has no addresses", func(t *testing.T) {
		t.Parallel()

		assert.Empty(t, NewEndpoints(task.Task{Slug: "nginx-xkfqz"}, ingressDomain))
	})
}

func TestNewContainer(t *testing.T) {
	t.Parallel()

	presented := NewContainer(task.Task{
		UUID:         "task-uuid",
		Name:         "nginx",
		Slug:         "nginx-xkfqz",
		Kind:         task.KindService,
		StackUUID:    "stack-uuid",
		ServiceName:  "web",
		OwnerUUID:    "owner-uuid",
		MaxRetries:   2,
		Retries:      1,
		CurrentState: task.Running,
		Image:        "nginx:1.27-alpine",
		ExposedPorts: []port.Port{80},
		Endpoints:    []task.Endpoint{{ContainerPort: 80, Host: "docker", HostPort: 32768}},
		Environment:  []string{"TZ=UTC"},
		Command:      []string{"nginx", "-g", "daemon off;"},
		WorkingDir:   "/app",
		ResourceLimits: task.ResourceLimits{
			Cpu:    0.5,
			Memory: 256 << 20,
			Disk:   1 << 30,
		},
	}, ingressDomain, NewOwners([]user.User{{UUID: "owner-uuid", Name: "Mahdi", Username: "mahdi", Avatar: "avatar-uuid"}}))

	assert.Equal(t, "task-uuid", presented.UUID)
	assert.Equal(t, "nginx", presented.Name)
	assert.Equal(t, "nginx-xkfqz", presented.Slug)
	assert.Equal(t, "running", presented.State)
	assert.Equal(t, "stack-uuid", presented.StackUUID)
	assert.Equal(t, "web", presented.ServiceName)
	assert.Equal(t, 0.5, presented.Limits.Cpu)
	assert.Equal(t, uint64(256<<20), presented.Limits.Memory)

	assert.Equal(t, 2, presented.MaxRetries, "what it is worth being asked for again")
	assert.Equal(t, 1, presented.Retries, "and how much of that has been used")

	assert.Equal(t, "owner-uuid", presented.Owner.UUID)
	assert.Equal(t, "Mahdi", presented.Owner.Name, "a container says who it belongs to")
	assert.Equal(t, "mahdi", presented.Owner.Username)

	require.Len(t, presented.Endpoints, 1)
	assert.Equal(t, "http://nginx-xkfqz.runner.localhost:8021", presented.Endpoints[0].URL)
}
