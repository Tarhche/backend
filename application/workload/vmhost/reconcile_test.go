package vmhost

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/khanzadimahdi/testproject/domain/workload/guest"
	"github.com/khanzadimahdi/testproject/domain/workload/network"
	"github.com/khanzadimahdi/testproject/domain/workload/vm"
	vmMock "github.com/khanzadimahdi/testproject/infrastructure/repository/mocks/workload/vm"
)

// redeploy is vmhost going away and another taking its place: the VMs and
// their machines stay as they are.
func (w *world) redeploy() {
	w.t.Helper()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	require.NoError(w.t, w.engine.Close(ctx))

	w.engine = w.newEngine()
}

// seqs is the numbers of every line a VM's task wrote, as vmhost kept them.
func (w *world) seqs(id string) []uint64 {
	w.t.Helper()

	var seqs []uint64
	require.NoError(w.t, w.engine.Logs(w.ctx, id, 0, time.Time{}, false, func(line vm.LogLine) error {
		seqs = append(seqs, line.Seq)

		return nil
	}))

	return seqs
}

// leftRunning is a VM an earlier vmhost was running, whose machine still runs
// with agent in it.
func (w *world) leftRunning(name string, agent *vmMock.FakeAgent) string {
	w.t.Helper()

	id := w.create(spec(name))

	_, err := w.states.Update(w.ctx, id, func(v *vm.VM) {
		v.State = vm.StateRunning
		v.Generation = 1
		v.Interfaces = []vm.Interface{{Network: network.IsolatedNetworkName, Device: "wkt-" + name, Address: "10.250.0.9/24"}}
	})
	require.NoError(w.t, err)

	w.hypervisor.Leave(vm.Machine{ID: id, Running: true, VsockPath: "fake://" + id + "/left/v.sock"}, agent)

	return id
}

func TestEngine_Reconcile(t *testing.T) {
	t.Parallel()

	t.Run("a vm an earlier vmhost was running is taken back, and what it wrote meanwhile is kept", func(t *testing.T) {
		t.Parallel()

		w := newWorld(t, func(uint64, guest.Process) vmMock.FakeRun {
			return vmMock.FakeRun{Lines: []string{"booted"}}
		})

		id := w.run(spec("service"))

		// booted twice, so what this boot writes is numbered after one line.
		require.NoError(t, w.engine.Restart(w.ctx, id))
		require.Eventually(t, func() bool { return len(w.output(id)) == 2 }, 5*time.Second, 5*time.Millisecond)
		require.Equal(t, uint64(1), w.vm(id).LogBase)

		w.redeploy()

		w.agent(id).Write(guest.StreamStdout, "while nobody was looking")

		require.NoError(t, w.engine.Reconcile(w.ctx))
		require.NotNil(t, w.engine.keeper(id), "it is looked after again")

		w.agent(id).Write(guest.StreamStderr, "and after")

		require.Eventually(t, func() bool { return len(w.output(id)) == 4 }, 5*time.Second, 5*time.Millisecond)
		assert.Equal(t, []string{"booted", "booted", "while nobody was looking", "and after"}, w.output(id))
		assert.Equal(t, []uint64{1, 2, 3, 4}, w.seqs(id))

		assert.Len(t, w.hypervisor.Booted(), 2, "nothing was booted to take it back")

		require.NoError(t, w.engine.Stop(w.ctx, id, time.Second))
		assert.Equal(t, vm.StateExited, w.vm(id).State)
	})

	t.Run("an agent this vmhost does not know is refused, and its vm is dead with the reason", func(t *testing.T) {
		t.Parallel()

		w := newWorld(t, nil)

		agent := vmMock.NewFakeAgent(nil).Running(1)
		agent.Refuse(errors.New("the agent speaks protocol 0, which this vmhost does not know"))

		id := w.leftRunning("old", agent)

		require.NoError(t, w.engine.Reconcile(w.ctx))

		dead := w.vm(id)
		assert.Equal(t, vm.StateDead, dead.State)
		assert.Equal(t, lostExitCode, dead.ExitCode)
		assert.Contains(t, dead.Reason, "protocol 0")
		assert.Empty(t, dead.Interfaces)

		_, err := w.hypervisor.Machine(w.ctx, id)
		assert.ErrorIs(t, err, vm.ErrNotFound, "its machine is ended")
	})

	t.Run("an agent that does not answer is looked after all the same, since its machine runs", func(t *testing.T) {
		t.Parallel()

		w := newWorld(t, nil)

		agent := vmMock.NewFakeAgent(nil).Running(1)
		agent.Hang()

		id := w.leftRunning("stuck", agent)

		require.NoError(t, w.engine.Reconcile(w.ctx))
		require.NotNil(t, w.engine.keeper(id))
		assert.Equal(t, vm.StateRunning, w.vm(id).State)

		require.NoError(t, w.engine.Kill(w.ctx, id), "one that does not answer is ended from outside")
		assert.Equal(t, vm.StateExited, w.vm(id).State)
	})

	t.Run("a boot that was not seen through is booted again", func(t *testing.T) {
		t.Parallel()

		w := newWorld(t, nil)

		// the machine booted, and its agent was never told anything.
		id := w.leftRunning("interrupted", vmMock.NewFakeAgent(nil))

		require.NoError(t, w.engine.Reconcile(w.ctx))

		running := w.vm(id)
		assert.Equal(t, vm.StateRunning, running.State)
		assert.Len(t, w.hypervisor.Booted(), 1)
		assert.NotNil(t, w.agent(id).Configured())
	})

	t.Run("a vm whose machine went away while nobody looked is dead, or booted again by its policy", func(t *testing.T) {
		t.Parallel()

		w := newWorld(t, nil)

		gone := w.create(spec("gone"))

		restarted := spec("restarted")
		restarted.RestartPolicy = "unless-stopped"
		again := w.create(restarted)

		for _, id := range []string{gone, again} {
			_, err := w.states.Update(w.ctx, id, func(v *vm.VM) {
				v.State = vm.StateRunning
				v.Interfaces = []vm.Interface{{Network: network.IsolatedNetworkName, Device: "wkt-" + id}}
			})
			require.NoError(t, err)
		}

		require.NoError(t, w.engine.Reconcile(w.ctx))

		dead := w.vm(gone)
		assert.Equal(t, vm.StateDead, dead.State)
		assert.Equal(t, lostExitCode, dead.ExitCode)
		assert.Empty(t, dead.Interfaces)

		running := w.vm(again)
		assert.Equal(t, vm.StateRunning, running.State)
		assert.Equal(t, uint(1), running.RestartCount)
		assert.NotNil(t, w.engine.keeper(again))
	})

	t.Run("a vm stopped on purpose stays stopped, whatever its policy", func(t *testing.T) {
		t.Parallel()

		w := newWorld(t, nil)

		s := spec("stopped")
		s.RestartPolicy = "always"

		id := w.run(s)
		require.NoError(t, w.engine.Stop(w.ctx, id, time.Second))

		w.redeploy()
		require.NoError(t, w.engine.Reconcile(w.ctx))

		assert.Equal(t, vm.StateExited, w.vm(id).State)
		assert.Len(t, w.hypervisor.Booted(), 1)
	})

	t.Run("machines nothing accounts for are ended", func(t *testing.T) {
		t.Parallel()

		w := newWorld(t, nil)

		orphan := vmMock.NewFakeAgent(nil).Running(1)
		w.hypervisor.Leave(vm.Machine{ID: "ffffffffffffffff", Running: true, VsockPath: "fake://orphan"}, orphan)

		exited := w.run(spec("exited"))
		require.NoError(t, w.engine.Stop(w.ctx, exited, time.Second))
		w.hypervisor.Leave(vm.Machine{ID: exited, Running: true, VsockPath: "fake://left-behind"}, vmMock.NewFakeAgent(nil).Running(1))

		require.NoError(t, w.engine.Reconcile(w.ctx))

		machines, err := w.hypervisor.Machines(w.ctx)
		require.NoError(t, err)
		assert.Empty(t, machines)
		assert.True(t, orphan.Gone())
		assert.Equal(t, vm.StateExited, w.vm(exited).State)
	})

	t.Run("taps and addresses nothing holds are given back", func(t *testing.T) {
		t.Parallel()

		w := newWorld(t, nil)

		running := w.run(spec("running"))

		_, err := w.fabric.Plug(w.ctx, "eeeeeeeeeeeeeeee", firstUID, []vm.Attachment{{Network: network.IsolatedNetworkName}})
		require.NoError(t, err)

		require.NoError(t, w.engine.Reconcile(w.ctx))

		assert.Equal(t, []string{running}, w.fabric.Retained())
		assert.Empty(t, w.fabric.Plugged("eeeeeeeeeeeeeeee"))
		assert.NotEmpty(t, w.fabric.Plugged(running))
	})

	t.Run("the firewall is put back every time, and vmhost is unhealthy while it cannot be", func(t *testing.T) {
		t.Parallel()

		w := newWorld(t, nil)

		require.NoError(t, w.engine.Reconcile(w.ctx))
		require.NoError(t, w.engine.Reconcile(w.ctx))
		assert.Equal(t, 2, w.fabric.Repairs())

		w.fabric.FailRepair(errors.New("iptables is gone"))

		assert.Error(t, w.engine.Reconcile(w.ctx))

		healthy, reason := w.engine.Health()
		assert.False(t, healthy)
		assert.Contains(t, reason, "iptables is gone")

		info, err := w.engine.Info(w.ctx)
		require.NoError(t, err)
		assert.False(t, info.Healthy)
		assert.Contains(t, info.Reason, "iptables is gone")

		w.fabric.FailRepair(nil)
		require.NoError(t, w.engine.Reconcile(w.ctx))

		healthy, _ = w.engine.Health()
		assert.True(t, healthy)
	})

	t.Run("the network vms route out through is made once", func(t *testing.T) {
		t.Parallel()

		w := newWorld(t, nil)
		w.fabric = vmMock.NewFakeFabric()
		w.engine = w.newEngine()

		require.NoError(t, w.engine.Reconcile(w.ctx))

		networks, err := w.fabric.Networks(w.ctx)
		require.NoError(t, err)
		require.Len(t, networks, 1)
		assert.Equal(t, vm.PublicNetwork, networks[0].Name)
		assert.True(t, networks[0].Masquerade)
	})

	t.Run("a hypervisor that cannot say what it holds makes vmhost unhealthy", func(t *testing.T) {
		t.Parallel()

		w := newWorld(t, nil)
		w.engine.hypervisor = blindHypervisor{FakeHypervisor: w.hypervisor}

		assert.Error(t, w.engine.Reconcile(w.ctx))

		healthy, reason := w.engine.Health()
		assert.False(t, healthy)
		assert.Contains(t, reason, "not implemented yet")
	})

	t.Run("images no vm boots are let go of, least recently used first, once they take more than they may", func(t *testing.T) {
		t.Parallel()

		w := newWorld(t, nil, func(c *Config) { c.ImageCacheMax = 2 << 20 })

		booted := w.create(spec("booted"))
		bootedDigest := w.vm(booted).ImageDigest

		now := time.Now()
		w.images.Add("oldest:1", vm.Image{Digest: "sha256:01", Size: 1 << 20, LastUsedAt: now.Add(-3 * time.Hour)})
		w.images.Add("older:1", vm.Image{Digest: "sha256:02", Size: 1 << 20, LastUsedAt: now.Add(-2 * time.Hour)})
		w.images.Add("newer:1", vm.Image{Digest: "sha256:03", Size: 1 << 20, LastUsedAt: now.Add(-time.Hour)})

		require.NoError(t, w.engine.Reconcile(w.ctx))

		images, err := w.engine.Images(w.ctx)
		require.NoError(t, err)

		var kept []string
		for _, image := range images {
			kept = append(kept, image.Digest)
		}

		assert.ElementsMatch(t, []string{bootedDigest, "sha256:03"}, kept)
	})

	t.Run("a store that lets go of images itself is told which images vms boot, and left to it", func(t *testing.T) {
		t.Parallel()

		w := newWorld(t, nil, func(c *Config) { c.ImageCacheMax = 1 })

		store := &pruningStore{FakeImageStore: w.images}
		w.engine.images = store

		first := w.vm(w.create(spec("first"))).ImageDigest

		other := spec("other")
		other.Image = "nginx:alpine"
		second := w.vm(w.create(other)).ImageDigest

		w.images.Add("unused:1", vm.Image{Digest: "sha256:01", Size: 1 << 20, LastUsedAt: time.Now().Add(-time.Hour)})

		require.NoError(t, w.engine.Reconcile(w.ctx))

		store.lock.Lock()
		defer store.lock.Unlock()

		require.Len(t, store.kept, 1)
		assert.ElementsMatch(t, []string{first, second}, store.kept[0])

		_, err := w.images.List(w.ctx)
		require.NoError(t, err)

		images, err := w.engine.Images(w.ctx)
		require.NoError(t, err)
		assert.Len(t, images, 3, "what to let go of is the store's to choose")
	})
}

// pruningStore is an image store that lets go of images itself, as the real
// one does, and keeps what it was told VMs boot.
type pruningStore struct {
	*vmMock.FakeImageStore

	lock sync.Mutex
	kept [][]string
}

func (s *pruningStore) Prune(ctx context.Context, keep []string) ([]vm.Image, error) {
	s.lock.Lock()
	defer s.lock.Unlock()

	s.kept = append(s.kept, keep)

	return nil, nil
}

// blindHypervisor cannot say which machines it holds, as the stub could not.
type blindHypervisor struct {
	*vmMock.FakeHypervisor
}

func (blindHypervisor) Machines(ctx context.Context) ([]vm.Machine, error) {
	return nil, errors.New("firecracker: not implemented yet")
}
