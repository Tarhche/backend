package workload

import (
	"context"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/danceable/container"
	"github.com/danceable/provider"
	"github.com/danceable/provider/adapters/danceable"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/khanzadimahdi/testproject/application/workload/microsandbox/runs"
	"github.com/khanzadimahdi/testproject/infrastructure/configs"
	fakes "github.com/khanzadimahdi/testproject/infrastructure/repository/mocks/workload/microsandbox"
	"github.com/khanzadimahdi/testproject/infrastructure/telemetry/profiler"
	"github.com/khanzadimahdi/testproject/infrastructure/workload/microsandbox/api"
)

// microsandboxContainer is a container holding what the provider needs from
// the providers before it: the configuration, a logger, the profiler, and a
// fake microsandbox in place of the SDK's.
func microsandboxContainer(t *testing.T, change func(*configs.WorkloadMicrosandbox)) (provider.Container, *fakes.FakeSandboxes) {
	t.Helper()

	c := danceable.New(container.New())

	microsandboxConfigs := configs.NewWorkloadMicrosandbox()
	microsandboxConfigs.StateDir = t.TempDir()
	microsandboxConfigs.MemoryBudget = 1 << 30
	microsandboxConfigs.PortBindAddress = "127.0.0.1"

	if change != nil {
		change(microsandboxConfigs)
	}

	fake := fakes.NewFakeSandboxes()

	require.NoError(t, c.Bind(func() *configs.WorkloadMicrosandbox { return microsandboxConfigs }, provider.Singleton()))
	require.NoError(t, c.Bind(func(string) *slog.Logger { return slog.New(slog.DiscardHandler) }, provider.Lazy()))
	require.NoError(t, c.Bind(func() *profiler.TracedProfiler { return profiler.NewTracedProfiler() }, provider.Singleton()))
	require.NoError(t, c.Bind(func() runs.Sandboxes { return fake }, provider.Singleton()))

	return c, fake
}

func TestMicrosandboxProvider(t *testing.T) {
	t.Parallel()

	t.Run("it wires a supervisor, the API and the healthcheck, over the microsandbox it is given", func(t *testing.T) {
		t.Parallel()

		c, _ := microsandboxContainer(t, nil)

		p := NewMicrosandboxProvider()

		require.NoError(t, p.Register(context.Background(), c))
		require.NoError(t, p.Boot(context.Background(), c))

		t.Cleanup(func() { assert.NoError(t, p.Terminate(context.Background())) })

		var supervisor *runs.Supervisor
		require.NoError(t, c.Resolve(&supervisor))

		var handler http.Handler
		require.NoError(t, c.Resolve(&handler))

		var health http.Handler
		require.NoError(t, c.Resolve(&health, provider.ResolveName(MicrosandboxHealth)))

		// not ready until it has been opened, which the healthcheck says.
		recorder := httptest.NewRecorder()
		health.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/health", nil))
		assert.Equal(t, http.StatusServiceUnavailable, recorder.Code)

		require.NoError(t, supervisor.Open(context.Background()))
		t.Cleanup(func() { _ = supervisor.Shutdown(context.Background()) })

		recorder = httptest.NewRecorder()
		health.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/health", nil))
		assert.Equal(t, http.StatusOK, recorder.Code)

		recorder = httptest.NewRecorder()
		handler.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, api.RouteInfo[len("GET "):], nil))
		assert.Equal(t, http.StatusOK, recorder.Code)
		assert.Contains(t, recorder.Body.String(), `"ready":true`)
	})

	t.Run("records and journals are kept under the state directory", func(t *testing.T) {
		t.Parallel()

		c, fake := microsandboxContainer(t, nil)
		fake.CacheImage("busybox", runs.ImageConfig{Cmd: []string{"sh"}})

		p := NewMicrosandboxProvider()
		require.NoError(t, p.Boot(context.Background(), c))

		var supervisor *runs.Supervisor
		require.NoError(t, c.Resolve(&supervisor))
		require.NoError(t, supervisor.Open(context.Background()))

		t.Cleanup(func() {
			_ = supervisor.Shutdown(context.Background())
			_ = p.Terminate(context.Background())
		})

		run, err := supervisor.Create(context.Background(), api.RunSpec{
			Node: "orchestrator-1", Name: "job", Image: "busybox", Memory: 64 << 20, Network: api.NetworkIsolated,
		})
		require.NoError(t, err)

		var microsandboxConfigs *configs.WorkloadMicrosandbox
		require.NoError(t, c.Resolve(&microsandboxConfigs))

		assert.FileExists(t, filepath.Join(microsandboxConfigs.StateDir, "runs", run.ID+".json"))
		assert.DirExists(t, filepath.Join(microsandboxConfigs.StateDir, "logs"))
	})

	t.Run("a container with no memory limit and no budget is refused", func(t *testing.T) {
		t.Parallel()

		c, _ := microsandboxContainer(t, func(c *configs.WorkloadMicrosandbox) { c.MemoryBudget = 0 })

		p := NewMicrosandboxProvider()
		p.cgroupRoot = t.TempDir()

		err := p.Boot(context.Background(), c)

		assert.ErrorContains(t, err, "no memory limit")
	})

	t.Run("the budget is worked out from the container's limit", func(t *testing.T) {
		t.Parallel()

		c, _ := microsandboxContainer(t, func(c *configs.WorkloadMicrosandbox) { c.MemoryBudget = 0 })

		root := t.TempDir()
		require.NoError(t, os.WriteFile(filepath.Join(root, "memory.max"), []byte("4294967296\n"), 0o600))

		p := NewMicrosandboxProvider()
		p.cgroupRoot = root

		require.NoError(t, p.Boot(context.Background(), c))
		t.Cleanup(func() { _ = p.Terminate(context.Background()) })
	})

	t.Run("a port range that is none is refused", func(t *testing.T) {
		t.Parallel()

		c, _ := microsandboxContainer(t, func(c *configs.WorkloadMicrosandbox) { c.PortRange = "nonsense" })

		assert.Error(t, NewMicrosandboxProvider().Boot(context.Background(), c))
	})
}

func TestContainerMemoryLimit(t *testing.T) {
	t.Parallel()

	write := func(t *testing.T, root, file, content string) {
		t.Helper()

		path := filepath.Join(root, file)
		require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o700))
		require.NoError(t, os.WriteFile(path, []byte(content), 0o600))
	}

	tests := []struct {
		name  string
		files map[string]string
		want  uint64
	}{
		{"cgroup v2 with a limit", map[string]string{"memory.max": "17179869184\n"}, 16 << 30},
		{"cgroup v2 with none", map[string]string{"memory.max": "max\n"}, 0},
		{"cgroup v1 with a limit", map[string]string{"memory/memory.limit_in_bytes": "1073741824\n"}, 1 << 30},
		{"cgroup v1 with none", map[string]string{"memory/memory.limit_in_bytes": "9223372036854771712\n"}, 0},
		{"no cgroup at all", nil, 0},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			root := t.TempDir()
			for file, content := range test.files {
				write(t, root, file, content)
			}

			limit, err := containerMemoryLimit(root)

			require.NoError(t, err)
			assert.Equal(t, test.want, limit)
		})
	}

	t.Run("a limit that is no number is an error", func(t *testing.T) {
		t.Parallel()

		root := t.TempDir()
		write(t, root, "memory.max", "lots")

		_, err := containerMemoryLimit(root)

		assert.Error(t, err)
	})
}

func TestServiceVersion(t *testing.T) {
	t.Parallel()

	assert.NotEmpty(t, serviceVersion())
}
