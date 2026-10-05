package vmhost

import (
	"context"
	"errors"
	"log/slog"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/khanzadimahdi/testproject/domain/workload/vm"
	"github.com/khanzadimahdi/testproject/infrastructure/configs"
	"github.com/khanzadimahdi/testproject/infrastructure/workload/vm/memory"
	"github.com/khanzadimahdi/testproject/infrastructure/workload/vm/microsandbox"
)

const gibibyte = 1 << 30

// host is a Host that says what it is told to.
type host struct {
	cpus   uint
	memory uint64
	err    error
}

func (h host) CPUs() uint {
	return h.cpus
}

func (h host) Memory() (uint64, error) {
	return h.memory, h.err
}

// settings are a vmhost's, as its compose file gives them, with the budget
// left to the vmhost.
func settings() *configs.WorkloadVMHost {
	s := configs.NewWorkloadVMHost()
	s.AdvertiseHost = "10.89.1.10"
	s.OrchestratorAddress = "10.89.1.2"

	return s
}

func TestOptions(t *testing.T) {
	t.Parallel()

	logger := slog.New(slog.DiscardHandler)

	t.Run("a vmhost's settings, the budget left to it", func(t *testing.T) {
		t.Parallel()

		options, err := Options(settings(), host{cpus: 6, memory: 8 * gibibyte}, logger)
		require.NoError(t, err)

		assert.Equal(t, microsandbox.Options{
			Home:                "/data/msb",
			BindAddress:         "10.89.1.10",
			OrchestratorAddress: "10.89.1.2",
			FirstPort:           20000,
			LastPort:            29999,
			DockerImage:         "docker:29-dind",
			Capacity: vm.Resources{
				CPUs:   6,
				Memory: 8*gibibyte/10*8 - 512<<20,
				Disk:   200 * gibibyte,
			},
			MaxConcurrentBoots: 4,
			Logger:             logger,
		}, options)
	})

	t.Run("a budget that is said is the budget", func(t *testing.T) {
		t.Parallel()

		s := settings()
		s.CPUs = 32
		s.Memory = 64 * gibibyte
		s.Disk = 1000 * gibibyte
		s.PortRange = "30000-30099"
		s.AdvertiseHost = "fd00::10"
		s.OrchestratorAddress = "fd00::2"

		options, err := Options(s, host{err: errors.New("not to be asked")}, logger)
		require.NoError(t, err)

		assert.Equal(t, vm.Resources{CPUs: 32, Memory: 64 * gibibyte, Disk: 1000 * gibibyte}, options.Capacity)
		assert.Equal(t, []uint{30000, 30099}, []uint{uint(options.FirstPort), uint(options.LastPort)})
		assert.Equal(t, "fd00::10", options.BindAddress)
	})

	testcases := []struct {
		name   string
		change func(s *configs.WorkloadVMHost)
		host   host

		wantSaid string
	}{
		{name: "no advertise host", change: func(s *configs.WorkloadVMHost) { s.AdvertiseHost = "" }, wantSaid: "WORKLOAD_VMHOST_ADVERTISE_HOST"},
		{name: "an advertise host that is a name", change: func(s *configs.WorkloadVMHost) { s.AdvertiseHost = "workload-vmhost-01" }, wantSaid: `got "workload-vmhost-01"`},
		{name: "no orchestrator", change: func(s *configs.WorkloadVMHost) { s.OrchestratorAddress = "" }, wantSaid: "WORKLOAD_VMHOST_ORCHESTRATOR_IP"},
		{name: "an orchestrator that is a name", change: func(s *configs.WorkloadVMHost) { s.OrchestratorAddress = "workload-orchestrator-01" }, wantSaid: "WORKLOAD_VMHOST_ORCHESTRATOR_IP"},
		{name: "a port range that is one port", change: func(s *configs.WorkloadVMHost) { s.PortRange = "20000" }, wantSaid: "first-last"},
		{name: "a port range backwards", change: func(s *configs.WorkloadVMHost) { s.PortRange = "29999-20000" }, wantSaid: "after it ends"},
		{name: "a port past the last", change: func(s *configs.WorkloadVMHost) { s.PortRange = "20000-70000" }, wantSaid: "1 to 65535"},
		{name: "a port zero", change: func(s *configs.WorkloadVMHost) { s.PortRange = "0-10" }, wantSaid: "1 to 65535"},
		{name: "no home", change: func(s *configs.WorkloadVMHost) { s.Home = "" }, wantSaid: "MSB_HOME"},
		{name: "no docker image", change: func(s *configs.WorkloadVMHost) { s.DockerImage = "" }, wantSaid: "WORKLOAD_VMHOST_DOCKER_IMAGE"},
		{name: "no disk", change: func(s *configs.WorkloadVMHost) { s.Disk = 0 }, wantSaid: "WORKLOAD_VMHOST_DISK"},
		{name: "no boots at all", change: func(s *configs.WorkloadVMHost) { s.MaxConcurrentBoots = 0 }, wantSaid: "WORKLOAD_VMHOST_MAX_CONCURRENT_BOOTS"},
		{name: "a container too small to leave vms anything", host: host{cpus: 1, memory: 512 << 20}, wantSaid: "leaves VMs nothing"},
		{name: "a container whose memory cannot be read", host: host{cpus: 1, err: os.ErrNotExist}, wantSaid: "WORKLOAD_VMHOST_MEMORY has to be said"},
	}

	for _, tc := range testcases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			s := settings()
			if tc.change != nil {
				tc.change(s)
			}

			h := tc.host
			if h == (host{}) {
				h = host{cpus: 4, memory: 8 * gibibyte}
			}

			_, err := Options(s, h, logger)
			assert.ErrorContains(t, err, tc.wantSaid)
		})
	}
}

// laidOut is a filesystem root holding files, as a container's /proc and
// /sys hold them.
func laidOut(t *testing.T, files map[string]string) Machine {
	t.Helper()

	root := t.TempDir()

	for name, content := range files {
		path := filepath.Join(root, name)
		require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o755))
		require.NoError(t, os.WriteFile(path, []byte(content), 0o644))
	}

	return NewMachine(root)
}

const meminfo = "MemTotal:       16384000 kB\nMemFree:         1000000 kB\n"

func TestMachine(t *testing.T) {
	t.Parallel()

	testcases := []struct {
		name  string
		files map[string]string

		wantCPUs   uint
		wantMemory uint64
	}{
		{
			name: "cgroup v2, limited",
			files: map[string]string{
				"sys/fs/cgroup/cpu.max":    "150000 100000\n",
				"sys/fs/cgroup/memory.max": "8589934592\n",
				"proc/meminfo":             meminfo,
			},
			wantCPUs:   2,
			wantMemory: 8 * gibibyte,
		},
		{
			name: "cgroup v2, unlimited",
			files: map[string]string{
				"sys/fs/cgroup/cpu.max":    "max 100000\n",
				"sys/fs/cgroup/memory.max": "max\n",
				"proc/meminfo":             meminfo,
			},
			wantCPUs:   uint(runtime.NumCPU()),
			wantMemory: 16384000 << 10,
		},
		{
			name: "cgroup v1, limited",
			files: map[string]string{
				"sys/fs/cgroup/cpu/cpu.cfs_quota_us":         "400000\n",
				"sys/fs/cgroup/cpu/cpu.cfs_period_us":        "100000\n",
				"sys/fs/cgroup/memory/memory.limit_in_bytes": "4294967296\n",
				"proc/meminfo": meminfo,
			},
			wantCPUs:   4,
			wantMemory: 4 * gibibyte,
		},
		{
			name: "cgroup v1, unlimited",
			files: map[string]string{
				"sys/fs/cgroup/cpu/cpu.cfs_quota_us":         "-1\n",
				"sys/fs/cgroup/cpu/cpu.cfs_period_us":        "100000\n",
				"sys/fs/cgroup/memory/memory.limit_in_bytes": "9223372036854771712\n",
				"proc/meminfo": meminfo,
			},
			wantCPUs:   uint(runtime.NumCPU()),
			wantMemory: 16384000 << 10,
		},
		{
			name:       "no cgroup at all",
			files:      map[string]string{"proc/meminfo": meminfo},
			wantCPUs:   uint(runtime.NumCPU()),
			wantMemory: 16384000 << 10,
		},
	}

	for _, tc := range testcases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			machine := laidOut(t, tc.files)

			assert.Equal(t, tc.wantCPUs, machine.CPUs())

			memory, err := machine.Memory()
			require.NoError(t, err)
			assert.Equal(t, tc.wantMemory, memory)
		})
	}

	t.Run("a machine that says nothing of its memory", func(t *testing.T) {
		t.Parallel()

		_, err := laidOut(t, map[string]string{"proc/meminfo": "MemFree: 1 kB\n"}).Memory()
		assert.Error(t, err)

		_, err = laidOut(t, nil).Memory()
		assert.Error(t, err)
	})
}

// stopping is an engine that counts how often it was shut down.
type stopping struct {
	vm.Engine

	shutdowns int
}

func (s *stopping) Shutdown(context.Context) error {
	s.shutdowns++

	return nil
}

func TestEngine_ShutdownOnce(t *testing.T) {
	t.Parallel()

	stopped := &stopping{Engine: memory.New()}
	e := &engine{Engine: stopped}

	require.NoError(t, e.Shutdown(t.Context()))
	require.NoError(t, e.Shutdown(t.Context()), "the provider, after the command already did")

	assert.Equal(t, 1, stopped.shutdowns)

	p := &vmhostProvider{engine: e}
	require.NoError(t, p.Terminate(t.Context()))
	assert.Equal(t, 1, stopped.shutdowns)

	require.NoError(t, (&vmhostProvider{}).Terminate(t.Context()), "a provider that never made an engine has nothing to stop")
}
