package vm

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/khanzadimahdi/testproject/domain/workload/guest"
)

func TestResolveProcess(t *testing.T) {
	t.Parallel()

	nginx := ImageConfig{
		Entrypoint: []string{"/docker-entrypoint.sh"},
		Cmd:        []string{"nginx", "-g", "daemon off;"},
		Env:        []string{"PATH=/usr/bin", "NGINX_VERSION=1.27"},
		WorkingDir: "/",
		User:       "nginx",
	}

	cases := []struct {
		name     string
		spec     Spec
		expected guest.Process
	}{
		{
			name: "a vm that names nothing runs what its image says",
			spec: Spec{},
			expected: guest.Process{
				Args:       []string{"/docker-entrypoint.sh", "nginx", "-g", "daemon off;"},
				Env:        []string{"PATH=/usr/bin", "NGINX_VERSION=1.27"},
				WorkingDir: "/",
				User:       "nginx",
			},
		},
		{
			name: "a command replaces the image's command and keeps its entrypoint",
			spec: Spec{Command: []string{"nginx", "-T"}},
			expected: guest.Process{
				Args:       []string{"/docker-entrypoint.sh", "nginx", "-T"},
				Env:        []string{"PATH=/usr/bin", "NGINX_VERSION=1.27"},
				WorkingDir: "/",
				User:       "nginx",
			},
		},
		{
			name: "an entrypoint replaces the image's, and its command with it",
			spec: Spec{Entrypoint: []string{"/bin/sh", "-c"}},
			expected: guest.Process{
				Args:       []string{"/bin/sh", "-c"},
				Env:        []string{"PATH=/usr/bin", "NGINX_VERSION=1.27"},
				WorkingDir: "/",
				User:       "nginx",
			},
		},
		{
			name: "an entrypoint and a command are taken together",
			spec: Spec{
				Entrypoint: []string{"/bin/sh", "-c"},
				Command:    []string{"echo hello"},
				Env:        []string{"NGINX_VERSION=1.28", "EXTRA=1"},
				WorkingDir: "/srv",
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
			t.Parallel()

			process, err := ResolveProcess(nginx, c.spec)

			require.NoError(t, err)
			assert.Equal(t, c.expected, process)
		})
	}

	t.Run("nothing to run is an invalid request", func(t *testing.T) {
		t.Parallel()

		_, err := ResolveProcess(ImageConfig{}, Spec{})

		assert.ErrorIs(t, err, ErrNoCommand)
		assert.ErrorIs(t, err, ErrInvalid)
	})
}

func TestMergeEnv(t *testing.T) {
	t.Parallel()

	assert.Equal(t,
		[]string{"PATH=/sbin", "HOME=/root", "EXTRA"},
		MergeEnv([]string{"PATH=/usr/bin", "HOME=/root"}, []string{"PATH=/sbin", "EXTRA"}),
	)
	assert.Empty(t, MergeEnv(nil, nil))
}

func TestMachineSize(t *testing.T) {
	t.Parallel()

	const minMemory = 128 << 20

	t.Run("a share of a CPU is a whole one, since a machine's CPUs are whole", func(t *testing.T) {
		t.Parallel()

		vcpus, _ := MachineSize(Resources{CPU: 0.5}, minMemory)
		assert.Equal(t, 1, vcpus)

		vcpus, _ = MachineSize(Resources{CPU: 1.5}, minMemory)
		assert.Equal(t, 2, vcpus)

		vcpus, _ = MachineSize(Resources{}, minMemory)
		assert.Equal(t, 1, vcpus)
	})

	t.Run("memory is rounded up to a MiB, and never less than the least a machine boots with", func(t *testing.T) {
		t.Parallel()

		_, memory := MachineSize(Resources{Memory: 200 << 20}, minMemory)
		assert.Equal(t, 200, memory)

		_, memory = MachineSize(Resources{Memory: 200<<20 + 1}, minMemory)
		assert.Equal(t, 201, memory)

		_, memory = MachineSize(Resources{Memory: 16 << 20}, minMemory)
		assert.Equal(t, 128, memory)

		_, memory = MachineSize(Resources{}, minMemory)
		assert.Equal(t, 256, memory)

		_, memory = MachineSize(Resources{}, 512<<20)
		assert.Equal(t, 512, memory, "a vm naming no memory gets the least a machine boots with when that is more")
	})

	assert.Equal(t, uint64(256<<20), MemoryBytes(256))
}

func TestScratchSize(t *testing.T) {
	t.Parallel()

	assert.Equal(t, uint64(DefaultScratch), ScratchSize(Resources{}))
	assert.Equal(t, uint64(MinScratch), ScratchSize(Resources{Disk: 1 << 20}))
	assert.Equal(t, uint64(256<<20), ScratchSize(Resources{Disk: 256 << 20}))
}
