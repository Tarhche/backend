package vmhost

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/khanzadimahdi/testproject/domain/workload/guest"
	"github.com/khanzadimahdi/testproject/domain/workload/network"
	"github.com/khanzadimahdi/testproject/domain/workload/vm"
	vmMock "github.com/khanzadimahdi/testproject/infrastructure/repository/mocks/workload/vm"
	"github.com/khanzadimahdi/testproject/infrastructure/repository/vmstate"
	"github.com/khanzadimahdi/testproject/infrastructure/workload/vmm/layout"
)

// mocked is an engine whose hypervisor, fabric, agent and output are mocks,
// for following a boot step by step, with a real record store and an image
// store that makes any reference an image.
type mocked struct {
	ctx     context.Context
	dataDir string
	engine  *Engine
	images  *vmMock.FakeImageStore

	hypervisor *vmMock.MockHypervisor
	fabric     *vmMock.MockFabric
	guests     *vmMock.MockGuestConnector
	client     *vmMock.MockGuestClient
	logs       *vmMock.MockLogStore
	writer     *vmMock.MockLogWriter
}

func newMocked(t *testing.T) *mocked {
	t.Helper()

	dataDir := t.TempDir()

	states, err := vmstate.Open(dataDir)
	require.NoError(t, err)

	m := &mocked{
		ctx:        context.Background(),
		dataDir:    dataDir,
		images:     vmMock.NewFakeImageStore(),
		hypervisor: &vmMock.MockHypervisor{},
		fabric:     &vmMock.MockFabric{},
		guests:     &vmMock.MockGuestConnector{},
		client:     &vmMock.MockGuestClient{},
		logs:       &vmMock.MockLogStore{},
		writer:     &vmMock.MockLogWriter{},
	}

	m.engine, err = New(testConfig(dataDir), Parts{
		Hypervisor: m.hypervisor,
		Images:     m.images,
		Fabric:     m.fabric,
		Guests:     m.guests,
		States:     states,
		Logs:       m.logs,
	}, discard())
	require.NoError(t, err)

	m.engine.timing = fastTiming()

	return m
}

// close lets go of the VMs, which ends every keeper.
func (m *mocked) close(t *testing.T) {
	t.Helper()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	require.NoError(t, m.engine.Close(ctx))
}

// blocked answers what a keeper waits on only once it is let go of.
func blocked(args mock.Arguments) {
	<-args.Get(0).(context.Context).Done()
}

func TestEngine_Start(t *testing.T) {
	t.Parallel()

	t.Run("a vm is plugged in, booted, told what it is, and its task started and looked after", func(t *testing.T) {
		t.Parallel()

		m := newMocked(t)

		s := spec("web")
		s.Networks = append(s.Networks, vm.Attachment{Network: vm.PublicNetwork, Gateway: true})

		id, err := m.engine.Create(m.ctx, s)
		require.NoError(t, err)

		image, err := m.images.Ensure(m.ctx, s.Image)
		require.NoError(t, err)

		plugged := []vm.Interface{
			{Network: network.IsolatedNetworkName, Device: "wkt0", MAC: "06:00:0a:fa:00:02", Address: "10.250.0.2/24"},
			{Network: vm.PublicNetwork, Device: "wkt1", MAC: "06:00:0a:fa:01:02", Address: "10.250.1.2/24", Gateway: "10.250.1.1"},
		}

		machine := vm.Machine{ID: id, Running: true, VsockPath: "/var/lib/workload-vmhost/j/" + id + "/root/run/v.sock"}

		m.fabric.On("Plug", mock.Anything, id, firstUID, s.Networks).Once().Return(plugged, nil)
		m.hypervisor.On("Terminate", mock.Anything, id).Return(nil)
		m.hypervisor.On("Boot", mock.Anything, vm.MachineSpec{
			ID:         id,
			VCPUs:      1,
			CPU:        0.5,
			MemoryMiB:  256,
			Kernel:     testConfig(m.dataDir).Kernel,
			Initrd:     testConfig(m.dataDir).Initrd,
			KernelArgs: guest.KernelArgs,
			Drives:     []vm.Drive{{Path: image.Root, ReadOnly: true}, {Path: layout.Scratch(m.dataDir, id)}},
			NICs:       []vm.NIC{{Device: "wkt0", MAC: "06:00:0a:fa:00:02"}, {Device: "wkt1", MAC: "06:00:0a:fa:01:02"}},
			UID:        firstUID,
		}).Once().Return(machine, nil)
		m.guests.On("Connect", machine.VsockPath).Once().Return(m.client)
		m.client.On("Ready", mock.Anything).Once().Return(nil)
		m.client.On("Configure", mock.Anything, mock.MatchedBy(func(config guest.Config) bool {
			return config.Hostname == "web" &&
				config.Root == guest.Root{Image: guest.ImageDevice, Scratch: guest.ScratchDevice} &&
				assert.ObjectsAreEqual([]guest.Interface{
					{MAC: "06:00:0a:fa:00:02", Address: "10.250.0.2/24"},
					{MAC: "06:00:0a:fa:01:02", Address: "10.250.1.2/24", Gateway: "10.250.1.1"},
				}, config.Interfaces) &&
				assert.ObjectsAreEqual([]string{"1.1.1.1", "9.9.9.9"}, config.Nameservers) &&
				!config.Now.IsZero()
		})).Once().Return(nil)
		m.logs.On("Writer", id).Once().Return(m.writer, nil)
		m.writer.On("Last").Return(uint64(7))
		m.writer.On("Changed").Return(make(chan struct{})).Maybe()

		started := time.Now().UTC().Truncate(time.Second)
		m.client.On("Start", mock.Anything, guest.Process{Args: []string{"/bin/sh", "-c", "exec httpd -f"}, Env: []string{"PATH=/usr/bin:/bin"}}).
			Once().Return(guest.Status{State: guest.StateRunning, Generation: 1, StartedAt: started}, nil)

		// the keeper: waiting for the task to end, and following what it writes.
		waiting := make(chan struct{})
		m.client.On("Wait", mock.Anything, uint64(1)).Run(func(args mock.Arguments) {
			close(waiting)
			blocked(args)
		}).Once().Return(guest.Status{}, context.Canceled)
		m.client.On("Logs", mock.Anything, uint64(0), true, mock.Anything).Run(blocked).Return(context.Canceled).Maybe()
		m.client.On("Close").Once().Return(nil)
		m.writer.On("Close").Once().Return(nil)

		require.NoError(t, m.engine.Start(m.ctx, id))

		running, err := m.engine.VM(m.ctx, id)
		require.NoError(t, err)

		assert.Equal(t, vm.StateRunning, running.State)
		assert.Equal(t, plugged, running.Interfaces)
		assert.Equal(t, uint64(1), running.Generation)
		assert.Equal(t, uint64(7), running.LogBase, "what this boot writes is numbered after what the vm wrote before")
		assert.Equal(t, started, running.StartedAt)
		assert.Equal(t, []uint16{80}, running.Endpoints())

		select {
		case <-waiting:
		case <-time.After(5 * time.Second):
			t.Fatal("nobody looks after the vm")
		}

		require.NotNil(t, m.engine.keeper(id))

		require.NoError(t, m.engine.Start(m.ctx, id), "starting a vm that runs is starting nothing")

		m.close(t)

		m.hypervisor.AssertExpectations(t)
		m.fabric.AssertExpectations(t)
		m.client.AssertExpectations(t)
		m.writer.AssertExpectations(t)
	})

	t.Run("a machine that does not come up is let go of, and the vm says why", func(t *testing.T) {
		t.Parallel()

		m := newMocked(t)

		s := spec("web")

		id, err := m.engine.Create(m.ctx, s)
		require.NoError(t, err)

		failed := errors.New("the agent never answered")

		m.fabric.On("Plug", mock.Anything, id, firstUID, s.Networks).Once().Return([]vm.Interface{{Network: network.IsolatedNetworkName, Device: "wkt0"}}, nil)
		m.fabric.On("Unplug", mock.Anything, id).Once().Return(nil)
		m.hypervisor.On("Terminate", mock.Anything, id).Twice().Return(nil)
		m.hypervisor.On("Boot", mock.Anything, mock.Anything).Once().Return(vm.Machine{ID: id, VsockPath: "/v.sock"}, nil)
		m.guests.On("Connect", "/v.sock").Once().Return(m.client)
		m.client.On("Ready", mock.Anything).Once().Return(failed)
		m.client.On("Close").Once().Return(nil)

		err = m.engine.Start(m.ctx, id)
		assert.ErrorIs(t, err, failed)

		v, err := m.engine.VM(m.ctx, id)
		require.NoError(t, err)

		assert.Equal(t, vm.StateCreated, v.State)
		assert.Contains(t, v.Reason, "did not come up")
		assert.Empty(t, v.Interfaces)
		assert.Nil(t, m.engine.keeper(id))

		m.hypervisor.AssertExpectations(t)
		m.fabric.AssertExpectations(t)
		m.client.AssertExpectations(t)
		m.logs.AssertNotCalled(t, "Writer", mock.Anything)
	})

	t.Run("a task that cannot be started has ended, as a container's whose command is not there", func(t *testing.T) {
		t.Parallel()

		m := newMocked(t)

		s := spec("web")
		s.Networks = nil

		id, err := m.engine.Create(m.ctx, s)
		require.NoError(t, err)

		failed := errors.New("no such file or directory")

		m.fabric.On("Unplug", mock.Anything, id).Once().Return(nil)
		m.hypervisor.On("Terminate", mock.Anything, id).Twice().Return(nil)
		m.hypervisor.On("Boot", mock.Anything, mock.MatchedBy(func(spec vm.MachineSpec) bool { return len(spec.NICs) == 0 })).
			Once().Return(vm.Machine{ID: id, VsockPath: "/v.sock"}, nil)
		m.guests.On("Connect", "/v.sock").Once().Return(m.client)
		m.client.On("Ready", mock.Anything).Once().Return(nil)
		m.client.On("Configure", mock.Anything, mock.MatchedBy(func(config guest.Config) bool {
			return len(config.Interfaces) == 0 && len(config.Nameservers) == 0
		})).Once().Return(nil)
		m.logs.On("Writer", id).Once().Return(m.writer, nil)
		m.client.On("Start", mock.Anything, mock.Anything).Once().Return(guest.Status{}, failed)
		m.client.On("Close").Once().Return(nil)
		m.writer.On("Close").Once().Return(nil)

		err = m.engine.Start(m.ctx, id)
		assert.ErrorIs(t, err, failed)

		v, err := m.engine.VM(m.ctx, id)
		require.NoError(t, err)

		assert.Equal(t, vm.StateExited, v.State)
		assert.Equal(t, 127, v.ExitCode)
		assert.Contains(t, v.Reason, "could not be started")

		m.hypervisor.AssertExpectations(t)
		m.client.AssertExpectations(t)
		m.writer.AssertExpectations(t)
		m.fabric.AssertNotCalled(t, "Plug", mock.Anything, mock.Anything, mock.Anything, mock.Anything)
	})

	t.Run("a vm that had ended says it is restarting while it boots again, from the same disks", func(t *testing.T) {
		t.Parallel()

		gate := make(chan struct{})

		w := newWorld(t, nil)
		hypervisor := &gatedHypervisor{FakeHypervisor: w.hypervisor}
		w.engine = w.newEngine()
		w.engine.hypervisor = hypervisor

		id := w.run(spec("web"))
		require.NoError(t, w.engine.Stop(w.ctx, id, time.Second))
		assert.Equal(t, vm.StateExited, w.vm(id).State)

		hypervisor.gate(gate)

		started := make(chan error, 1)
		go func() { started <- w.engine.Start(w.ctx, id) }()

		w.waitFor(id, inState(vm.StateRestarting))

		close(gate)
		require.NoError(t, <-started)

		running := w.vm(id)
		assert.Equal(t, vm.StateRunning, running.State)
		assert.False(t, running.Stopped)
		assert.Empty(t, running.Reason)

		booted := w.hypervisor.Booted()
		require.Len(t, booted, 2)
		assert.Equal(t, booted[0].Drives, booted[1].Drives, "booted again from the same disks")
	})

	t.Run("a vm that cannot be booted again has still ended", func(t *testing.T) {
		t.Parallel()

		w := newWorld(t, nil)

		id := w.run(spec("web"))
		require.NoError(t, w.engine.Stop(w.ctx, id, time.Second))

		failed := errors.New("no kvm")
		w.hypervisor.FailBoot(failed)

		assert.ErrorIs(t, w.engine.Start(w.ctx, id), failed)

		v := w.vm(id)
		assert.Equal(t, vm.StateExited, v.State)
		assert.Equal(t, 143, v.ExitCode)
		assert.Contains(t, v.Reason, "no kvm")
		assert.Empty(t, w.fabric.Plugged(id), "what it was given is given back")
	})

	t.Run("a vm that is not there is not found", func(t *testing.T) {
		t.Parallel()

		w := newWorld(t, nil)

		assert.ErrorIs(t, w.engine.Start(w.ctx, "0123456789abcdef"), vm.ErrNotFound)
	})
}

// gatedHypervisor holds every boot until its gate is opened.
type gatedHypervisor struct {
	*vmMock.FakeHypervisor

	held chan struct{}
}

func (h *gatedHypervisor) gate(gate chan struct{}) {
	h.held = gate
}

func (h *gatedHypervisor) Boot(ctx context.Context, spec vm.MachineSpec) (vm.Machine, error) {
	if h.held != nil {
		<-h.held
	}

	return h.FakeHypervisor.Boot(ctx, spec)
}
