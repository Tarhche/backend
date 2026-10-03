// Package vmhosttest makes vmhost engines for tests: engines made of
// in-memory parts that behave like the real ones (the fakes beside the mocks
// of domain/workload/vm), with real record and output stores on a directory of
// the test's own. The use cases' tests and the API's are written against it,
// as net/http's are against httptest.
package vmhosttest

import (
	"context"
	"io"
	"log/slog"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/khanzadimahdi/testproject/application/workload/vmhost"
	"github.com/khanzadimahdi/testproject/domain/workload/network"
	"github.com/khanzadimahdi/testproject/domain/workload/vm"
	vmMock "github.com/khanzadimahdi/testproject/infrastructure/repository/mocks/workload/vm"
	"github.com/khanzadimahdi/testproject/infrastructure/repository/vmstate"
	"github.com/khanzadimahdi/testproject/infrastructure/workload/vmm/layout"
	"github.com/khanzadimahdi/testproject/infrastructure/workload/vmm/output"
)

// Host is an engine and the fakes it is made of.
type Host struct {
	Engine     *vmhost.Engine
	Hypervisor *vmMock.FakeHypervisor
	Guests     *vmMock.FakeGuests
	Images     *vmMock.FakeImageStore
	Fabric     *vmMock.FakeFabric
	DataDir    string
}

// Config is a host with room for a few VMs: 4 GiB of memory, 16 CPUs, and
// users of their own for its machines.
func Config(dataDir string) vmhost.Config {
	return vmhost.Config{
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
		FirstUID:       1_000_000_000,
		UIDs:           16,
		Nameservers:    []string{"1.1.1.1"},
	}
}

// New is a host whose tasks do what script says (nil runs every task until it
// is stopped), with the networks the orchestrator makes before it makes VMs:
// the isolated one and the public one. The engine is closed when the test
// ends.
func New(t testing.TB, script vmMock.FakeScript, adjust ...func(*vmhost.Config)) *Host {
	t.Helper()

	dataDir := t.TempDir()

	states, err := vmstate.Open(dataDir)
	require.NoError(t, err)

	guests := vmMock.NewFakeGuests(script)

	host := &Host{
		Hypervisor: vmMock.NewFakeHypervisor(guests),
		Guests:     guests,
		Images:     vmMock.NewFakeImageStore(),
		Fabric:     vmMock.NewFakeFabric(),
		DataDir:    dataDir,
	}

	config := Config(dataDir)
	for _, change := range adjust {
		change(&config)
	}

	host.Engine, err = vmhost.New(config, vmhost.Parts{
		Hypervisor: host.Hypervisor,
		Images:     host.Images,
		Fabric:     host.Fabric,
		Guests:     guests,
		States:     states,
		Logs:       output.NewStore(dataDir, 1<<20),
		FreeSpace:  func() (uint64, error) { return 100 << 30, nil },
	}, slog.New(slog.NewTextHandler(io.Discard, nil)))
	require.NoError(t, err)

	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()

		_ = host.Engine.Close(ctx)
	})

	ctx := context.Background()

	_, err = host.Fabric.EnsureNetwork(ctx, network.IsolatedNetworkName, false)
	require.NoError(t, err)

	_, err = host.Fabric.EnsureNetwork(ctx, vm.PublicNetwork, true)
	require.NoError(t, err)

	require.NoError(t, host.Engine.Reconcile(ctx))

	return host
}

// Spec is a VM as the orchestrator asks for one: a task on the isolated
// network, serving on port 80, on node orchestrator-01.
func Spec(name string) vm.Spec {
	return vm.Spec{
		Name:         name,
		Hostname:     name,
		Image:        "alpine:3.20",
		Labels:       map[string]string{"node.name": "orchestrator-01", "task.slug": name},
		Command:      []string{"/bin/sh", "-c", "exec httpd -f"},
		Resources:    vm.Resources{CPU: 0.5, Memory: 256 << 20, Disk: 128 << 20},
		Networks:     []vm.Attachment{{Network: network.IsolatedNetworkName}},
		ExposedPorts: []uint16{80},
	}
}

// Create makes a VM.
func (h *Host) Create(t testing.TB, spec vm.Spec) string {
	t.Helper()

	id, err := h.Engine.Create(context.Background(), spec)
	require.NoError(t, err)

	return id
}

// Run makes a VM and starts it.
func (h *Host) Run(t testing.TB, spec vm.Spec) string {
	t.Helper()

	id := h.Create(t, spec)
	require.NoError(t, h.Engine.Start(context.Background(), id))

	return id
}

// VM is a VM as the engine holds it.
func (h *Host) VM(t testing.TB, id string) vm.VM {
	t.Helper()

	v, err := h.Engine.VM(context.Background(), id)
	require.NoError(t, err)

	return v
}

// Agent is the agent inside a VM's machine, as it is now.
func (h *Host) Agent(t testing.TB, id string) *vmMock.FakeAgent {
	t.Helper()

	machine, err := h.Hypervisor.Machine(context.Background(), id)
	require.NoError(t, err)

	agent := h.Guests.Agent(machine.VsockPath)
	require.NotNil(t, agent)

	return agent
}
