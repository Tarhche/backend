package vmhost

import (
	"context"
	"io"
	"log/slog"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/khanzadimahdi/testproject/domain/workload/network"
	"github.com/khanzadimahdi/testproject/domain/workload/runtime"
	"github.com/khanzadimahdi/testproject/domain/workload/vm"
	vmMock "github.com/khanzadimahdi/testproject/infrastructure/repository/mocks/workload/vm"
	"github.com/khanzadimahdi/testproject/infrastructure/repository/vmstate"
	"github.com/khanzadimahdi/testproject/infrastructure/workload/vmm/layout"
	"github.com/khanzadimahdi/testproject/infrastructure/workload/vmm/output"
)

const (
	firstUID = 1_000_000_000
	node     = "orchestrator-01"
)

func discard() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

// fastTiming is the engine's timing shrunk, so that tests do not wait on
// what the engine waits on.
func fastTiming() timing {
	return timing{
		boot:       2 * time.Second,
		adopt:      300 * time.Millisecond,
		settle:     300 * time.Millisecond,
		finalize:   2 * time.Second,
		retry:      10 * time.Millisecond,
		gone:       5 * time.Millisecond,
		drain:      time.Second,
		poll:       10 * time.Millisecond,
		imageGrace: 0,
		backoff:    func(uint) time.Duration { return 10 * time.Millisecond },
	}
}

func testConfig(dataDir string) Config {
	return Config{
		Version:        "test",
		ProcessMode:    "child",
		Architecture:   "arm64",
		Kernel:         "/var/lib/workload-vmhost/boot/vmlinux-0123456789abcdef",
		Initrd:         "/var/lib/workload-vmhost/boot/initrd-0123456789abcdef.cpio.gz",
		ScratchPath:    func(id string) string { return layout.Scratch(dataDir, id) },
		MaxMemory:      4 << 30,
		MinMemory:      128 << 20,
		MemoryOverhead: 64 << 20,
		MaxVMMemory:    2 << 30,
		MaxVMCPU:       2,
		HostCPUs:       4,
		CPUOvercommit:  4,
		DiskReserve:    1 << 30,
		DiskOvercommit: 3,
		FirstUID:       firstUID,
		UIDs:           16,
		Nameservers:    []string{"1.1.1.1", "9.9.9.9"},
	}
}

// spec is a VM as the orchestrator asks for one: a task on the isolated
// network, serving on port 80.
func spec(name string) vm.Spec {
	return vm.Spec{
		Name:         name,
		Hostname:     name,
		Image:        "alpine:3.20",
		Labels:       map[string]string{"node.name": node, "task.slug": name},
		Command:      []string{"/bin/sh", "-c", "exec httpd -f"},
		Resources:    vm.Resources{CPU: 0.5, Memory: 256 << 20, Disk: 128 << 20},
		Networks:     []vm.Attachment{{Network: network.IsolatedNetworkName}},
		ExposedPorts: []uint16{80},
	}
}

// world is an engine made of fakes that behave like the real parts, with
// real stores on a directory of its own: a host to drive VMs through whole
// lives on.
type world struct {
	t       *testing.T
	ctx     context.Context
	dataDir string

	engine     *Engine
	hypervisor *vmMock.FakeHypervisor
	guests     *vmMock.FakeGuests
	images     *vmMock.FakeImageStore
	fabric     *vmMock.FakeFabric
	states     *vmstate.Store
	logs       *output.Store

	free uint64
}

func newWorld(t *testing.T, script vmMock.FakeScript, adjust ...func(*Config)) *world {
	t.Helper()

	dataDir := t.TempDir()

	states, err := vmstate.Open(dataDir)
	require.NoError(t, err)

	guests := vmMock.NewFakeGuests(script)

	w := &world{
		t:          t,
		ctx:        context.Background(),
		dataDir:    dataDir,
		hypervisor: vmMock.NewFakeHypervisor(guests),
		guests:     guests,
		images:     vmMock.NewFakeImageStore(),
		fabric:     vmMock.NewFakeFabric(),
		states:     states,
		logs:       output.NewStore(dataDir, 1<<20),
		free:       100 << 30,
	}

	w.engine = w.newEngine(adjust...)

	// the networks the orchestrator makes before it makes VMs on them.
	_, err = w.fabric.EnsureNetwork(w.ctx, network.IsolatedNetworkName, false)
	require.NoError(t, err)
	_, err = w.fabric.EnsureNetwork(w.ctx, vm.PublicNetwork, true)
	require.NoError(t, err)

	return w
}

// newEngine is another vmhost on the same host: the same parts and stores, as
// a vmhost that is redeployed finds them.
func (w *world) newEngine(adjust ...func(*Config)) *Engine {
	w.t.Helper()

	config := testConfig(w.dataDir)
	for _, change := range adjust {
		change(&config)
	}

	engine, err := New(config, Parts{
		Hypervisor: w.hypervisor,
		Images:     w.images,
		Fabric:     w.fabric,
		Guests:     w.guests,
		States:     w.states,
		Logs:       w.logs,
		FreeSpace:  func() (uint64, error) { return w.free, nil },
	}, discard())
	require.NoError(w.t, err)

	engine.timing = fastTiming()

	w.t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		_ = engine.Close(ctx)
	})

	return engine
}

func (w *world) create(s vm.Spec) string {
	w.t.Helper()

	id, err := w.engine.Create(w.ctx, s)
	require.NoError(w.t, err)

	return id
}

func (w *world) run(s vm.Spec) string {
	w.t.Helper()

	id := w.create(s)
	require.NoError(w.t, w.engine.Start(w.ctx, id))

	return id
}

func (w *world) vm(id string) vm.VM {
	w.t.Helper()

	v, err := w.engine.VM(w.ctx, id)
	require.NoError(w.t, err)

	return v
}

// waitFor waits for a VM to be what until says, and says what it was.
func (w *world) waitFor(id string, until func(vm.VM) bool) vm.VM {
	w.t.Helper()

	var last vm.VM

	require.Eventually(w.t, func() bool {
		v, err := w.engine.VM(w.ctx, id)
		if err != nil {
			return false
		}

		last = v

		return until(v)
	}, 5*time.Second, 5*time.Millisecond, "the vm never got there")

	return last
}

func inState(state vm.State) func(vm.VM) bool {
	return func(v vm.VM) bool { return v.State == state }
}

// agent is the agent inside a VM's machine, as it is now.
func (w *world) agent(id string) *vmMock.FakeAgent {
	w.t.Helper()

	machine, err := w.hypervisor.Machine(w.ctx, id)
	require.NoError(w.t, err)

	agent := w.guests.Agent(machine.VsockPath)
	require.NotNil(w.t, agent)

	return agent
}

// output is every line a VM's task wrote, as vmhost kept it.
func (w *world) output(id string) []string {
	w.t.Helper()

	var lines []string
	require.NoError(w.t, w.engine.Logs(w.ctx, id, 0, time.Time{}, false, func(line vm.LogLine) error {
		lines = append(lines, line.Content)

		return nil
	}))

	return lines
}

func TestNew(t *testing.T) {
	t.Parallel()

	t.Run("an engine is not made without its parts", func(t *testing.T) {
		t.Parallel()

		_, err := New(testConfig(t.TempDir()), Parts{}, discard())

		assert.ErrorIs(t, err, errMissingPart)
	})

	t.Run("an engine is unhealthy until it has looked at its machines", func(t *testing.T) {
		t.Parallel()

		w := newWorld(t, nil)

		healthy, reason := w.engine.Health()
		assert.False(t, healthy)
		assert.NotEmpty(t, reason)

		require.NoError(t, w.engine.Reconcile(w.ctx))

		healthy, reason = w.engine.Health()
		assert.True(t, healthy)
		assert.Empty(t, reason)
	})
}

func TestEngine_Info(t *testing.T) {
	t.Parallel()

	w := newWorld(t, nil)
	require.NoError(t, w.engine.Reconcile(w.ctx))

	w.run(spec("running"))
	w.create(spec("created"))

	stopped := w.run(spec("stopped"))
	require.NoError(t, w.engine.Stop(w.ctx, stopped, time.Second))

	readOnly := spec("read-only")
	readOnly.ReadOnly = true
	w.create(readOnly)

	info, err := w.engine.Info(w.ctx)
	require.NoError(t, err)

	assert.Equal(t, "test", info.Version)
	assert.Equal(t, "fake", info.Hypervisor)
	assert.Equal(t, "0.0.0", info.HypervisorVersion)
	assert.Equal(t, "1", info.GuestVersion)
	assert.Equal(t, "arm64", info.Architecture)
	assert.Equal(t, "child", info.ProcessMode)
	assert.True(t, info.Healthy)

	assert.Equal(t, runtime.Capabilities{
		Isolation:       runtime.IsolationMicroVM,
		NetworkPolicies: []network.Policy{network.PolicyNone, network.PolicyIsolated, network.PolicyPublic},
		StackNetworks:   true,
		ReadOnlyRoot:    true,
		DiskLimit:       true,
		TTY:             true,
		RestartPolicies: []string{"no", "always", "on-failure", "unless-stopped"},
		MinMemory:       128 << 20,
		MaxMemory:       2 << 30,
		MaxCPU:          2,
		Architectures:   []string{"arm64"},
	}, info.Capabilities)

	// the running, the created and the read-only VM hold their memory and
	// CPUs; the stopped one holds only its scratch disk.
	assert.Equal(t, runtime.Capacity{
		CPU:             16,
		AllocatedCPU:    3,
		Memory:          4 << 30,
		AllocatedMemory: 3 * (256 + 64) << 20,
		Disk:            300 << 30,
		AllocatedDisk:   3 * 128 << 20,
		Reserved:        true,
	}, info.Capacity)
}
