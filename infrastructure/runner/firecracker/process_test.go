package firecracker

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/khanzadimahdi/testproject/domain/runner/task"
	"github.com/khanzadimahdi/testproject/infrastructure/runner/firecracker/guest"
	"github.com/khanzadimahdi/testproject/infrastructure/runner/firecracker/image"
)

func TestResolveProcess(t *testing.T) {
	nginx := image.Config{
		Entrypoint: []string{"/docker-entrypoint.sh"},
		Cmd:        []string{"nginx", "-g", "daemon off;"},
		Env:        []string{"PATH=/usr/bin", "NGINX_VERSION=1.27"},
		WorkingDir: "/",
		User:       "nginx",
	}

	cases := []struct {
		name      string
		execution task.Execution
		expected  guest.Process
	}{
		{
			name:      "a task that names nothing runs what its image says",
			execution: task.Execution{},
			expected: guest.Process{
				Args:       []string{"/docker-entrypoint.sh", "nginx", "-g", "daemon off;"},
				Env:        []string{"PATH=/usr/bin", "NGINX_VERSION=1.27"},
				WorkingDir: "/",
				User:       "nginx",
			},
		},
		{
			name:      "a command replaces the image's command and keeps its entrypoint",
			execution: task.Execution{Command: []string{"nginx", "-T"}},
			expected: guest.Process{
				Args:       []string{"/docker-entrypoint.sh", "nginx", "-T"},
				Env:        []string{"PATH=/usr/bin", "NGINX_VERSION=1.27"},
				WorkingDir: "/",
				User:       "nginx",
			},
		},
		{
			name:      "an entrypoint replaces the image's, and its command with it",
			execution: task.Execution{Entrypoint: []string{"/bin/sh", "-c"}},
			expected: guest.Process{
				Args:       []string{"/bin/sh", "-c"},
				Env:        []string{"PATH=/usr/bin", "NGINX_VERSION=1.27"},
				WorkingDir: "/",
				User:       "nginx",
			},
		},
		{
			name: "an entrypoint and a command are taken together",
			execution: task.Execution{
				Entrypoint:       []string{"/bin/sh", "-c"},
				Command:          []string{"echo hello"},
				Environment:      []string{"NGINX_VERSION=1.28", "EXTRA=1"},
				WorkingDirectory: "/srv",
			},
			expected: guest.Process{
				Args:       []string{"/bin/sh", "-c", "echo hello"},
				Env:        []string{"PATH=/usr/bin", "NGINX_VERSION=1.28", "EXTRA=1"},
				WorkingDir: "/srv",
				User:       "nginx",
			},
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			process, err := resolveProcess(nginx, &c.execution)

			require.NoError(t, err)
			assert.Equal(t, c.expected, process)
		})
	}

	t.Run("nothing to run is refused", func(t *testing.T) {
		_, err := resolveProcess(image.Config{}, &task.Execution{})

		assert.ErrorIs(t, err, errNoCommand)
	})
}

func TestResources(t *testing.T) {
	t.Run("a share of a CPU is a whole one used that share of the time", func(t *testing.T) {
		vcpus, quota, _ := resources(task.ResourceLimits{Cpu: 0.5})

		assert.Equal(t, 1, vcpus)
		assert.Equal(t, 0.5, quota)

		vcpus, quota, _ = resources(task.ResourceLimits{Cpu: 1.5})

		assert.Equal(t, 2, vcpus)
		assert.Equal(t, 1.5, quota)
	})

	t.Run("memory is in bytes, and a machine is never given less than it boots in", func(t *testing.T) {
		_, _, memory := resources(task.ResourceLimits{Memory: 200 << 20})
		assert.Equal(t, 200, memory)

		_, _, memory = resources(task.ResourceLimits{Memory: 16 << 20})
		assert.Equal(t, minimumMemoryMiB, memory)

		_, _, memory = resources(task.ResourceLimits{})
		assert.Equal(t, defaultMemoryMiB, memory)
	})

	t.Run("a scratch disk is as large as the task may write, and never too small to make", func(t *testing.T) {
		assert.Equal(t, int64(defaultScratchBytes), scratchBytes(task.ResourceLimits{}))
		assert.Equal(t, int64(minimumScratchBytes), scratchBytes(task.ResourceLimits{Disk: 1 << 20}))
		assert.Equal(t, int64(256<<20), scratchBytes(task.ResourceLimits{Disk: 256 << 20}))
	})
}

func TestRestartPolicy(t *testing.T) {
	cases := []struct {
		policy   string
		exitCode int
		restarts uint
		stopped  bool
		expected bool
	}{
		{policy: "", exitCode: 1, expected: false},
		{policy: "no", exitCode: 1, expected: false},
		{policy: "always", exitCode: 0, expected: true},
		{policy: "always", exitCode: 1, stopped: true, expected: false},
		{policy: "unless-stopped", exitCode: 0, expected: true},
		{policy: "on-failure", exitCode: 0, expected: false},
		{policy: "on-failure", exitCode: 1, restarts: 100, expected: true},
		{policy: "on-failure:3", exitCode: 1, restarts: 2, expected: true},
		{policy: "on-failure:3", exitCode: 1, restarts: 3, expected: false},
	}

	for _, c := range cases {
		assert.Equal(t, c.expected, parsePolicy(c.policy).restarts(c.exitCode, c.restarts, c.stopped), "%+v", c)
	}

	t.Run("a task ending again is waited on longer each time, up to a minute", func(t *testing.T) {
		assert.Equal(t, 100*time.Millisecond, backoff(0))
		assert.Equal(t, 400*time.Millisecond, backoff(2))
		assert.Equal(t, time.Minute, backoff(50))
	})
}
