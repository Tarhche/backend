package vmhost

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/khanzadimahdi/testproject/domain/workload/guest"
	"github.com/khanzadimahdi/testproject/domain/workload/vm"
	vmMock "github.com/khanzadimahdi/testproject/infrastructure/repository/mocks/workload/vm"
	"github.com/khanzadimahdi/testproject/infrastructure/repository/vmstate"
	"github.com/khanzadimahdi/testproject/infrastructure/workload/vmm/layout"
)

func TestEngine_Create(t *testing.T) {
	t.Parallel()

	ctx := context.Background()

	t.Run("a vm is made from its image, with a scratch disk and a host user of its own, and nothing is booted", func(t *testing.T) {
		t.Parallel()

		dataDir := t.TempDir()

		states, err := vmstate.Open(dataDir)
		require.NoError(t, err)

		var (
			hypervisor vmMock.MockHypervisor
			images     vmMock.MockImageStore
			fabric     vmMock.MockFabric
			guests     vmMock.MockGuestConnector
			logs       vmMock.MockLogStore
		)

		image := vm.Image{
			Reference: "alpine:3.20",
			Digest:    "sha256:0f0f",
			Root:      "/var/lib/workload-vmhost/images/sha256:0f0f/rootfs.squashfs",
			Config: vm.ImageConfig{
				Entrypoint: []string{"/docker-entrypoint.sh"},
				Cmd:        []string{"sh"},
				Env:        []string{"PATH=/usr/bin:/bin"},
				User:       "nobody",
			},
		}

		images.On("Ensure", mock.Anything, "alpine:3.20").Once().Return(image, nil)
		images.On("MakeScratch", mock.Anything, mock.Anything, uint64(128<<20)).Once().Return(nil)
		defer images.AssertExpectations(t)

		engine, err := New(testConfig(dataDir), Parts{
			Hypervisor: &hypervisor,
			Images:     &images,
			Fabric:     &fabric,
			Guests:     &guests,
			States:     states,
			Logs:       &logs,
		}, discard())
		require.NoError(t, err)

		s := spec("web")
		s.Env = []string{"GREETING=hello"}

		id, err := engine.Create(ctx, s)
		require.NoError(t, err)
		assert.True(t, vm.IsID(id))

		images.AssertCalled(t, "MakeScratch", mock.Anything, layout.Scratch(dataDir, id), uint64(128<<20))

		made, err := engine.VM(ctx, id)
		require.NoError(t, err)

		assert.Equal(t, vm.StateCreated, made.State)
		assert.Equal(t, s, made.Spec)
		assert.Equal(t, "sha256:0f0f", made.ImageDigest)
		assert.Equal(t, guest.Process{
			Args: []string{"/docker-entrypoint.sh", "/bin/sh", "-c", "exec httpd -f"},
			Env:  []string{"PATH=/usr/bin:/bin", "GREETING=hello"},
			User: "nobody",
		}, made.Process)
		assert.Equal(t, 1, made.VCPUs)
		assert.Equal(t, 256, made.MemoryMiB)
		assert.Equal(t, firstUID, made.UID)
		assert.False(t, made.CreatedAt.IsZero())

		hypervisor.AssertNotCalled(t, "Boot", mock.Anything, mock.Anything)
		fabric.AssertNotCalled(t, "Plug", mock.Anything, mock.Anything, mock.Anything, mock.Anything)
	})

	t.Run("a scratch disk that cannot be made leaves nothing behind", func(t *testing.T) {
		t.Parallel()

		dataDir := t.TempDir()

		states, err := vmstate.Open(dataDir)
		require.NoError(t, err)

		var images vmMock.MockImageStore

		failed := errors.New("mke2fs failed")

		images.On("Ensure", mock.Anything, "alpine:3.20").Once().Return(vm.Image{Digest: "sha256:0f0f", Config: vm.ImageConfig{Cmd: []string{"sh"}}}, nil)
		images.On("MakeScratch", mock.Anything, mock.Anything, mock.Anything).Once().Return(failed)

		engine, err := New(testConfig(dataDir), Parts{
			Hypervisor: &vmMock.MockHypervisor{},
			Images:     &images,
			Fabric:     &vmMock.MockFabric{},
			Guests:     &vmMock.MockGuestConnector{},
			States:     states,
			Logs:       &vmMock.MockLogStore{},
		}, discard())
		require.NoError(t, err)

		_, err = engine.Create(ctx, spec("web"))
		assert.ErrorIs(t, err, failed)

		all, err := states.All(ctx)
		require.NoError(t, err)
		assert.Empty(t, all)
	})

	t.Run("a read-only vm has no scratch disk", func(t *testing.T) {
		t.Parallel()

		w := newWorld(t, nil)

		s := spec("read-only")
		s.ReadOnly = true

		id := w.create(s)

		_, made := w.images.Scratch(layout.Scratch(w.dataDir, id))
		assert.False(t, made)
	})

	t.Run("every vm runs as a host user of its own, and one let go of is taken again", func(t *testing.T) {
		t.Parallel()

		w := newWorld(t, nil)

		first := w.create(spec("first"))
		second := w.create(spec("second"))
		third := w.create(spec("third"))

		assert.Equal(t, firstUID, w.vm(first).UID)
		assert.Equal(t, firstUID+1, w.vm(second).UID)
		assert.Equal(t, firstUID+2, w.vm(third).UID)

		require.NoError(t, w.engine.Delete(w.ctx, second))

		assert.Equal(t, firstUID+1, w.vm(w.create(spec("fourth"))).UID)
	})

	t.Run("without users of their own, machines run as vmhost", func(t *testing.T) {
		t.Parallel()

		w := newWorld(t, nil, func(c *Config) { c.UIDs = 0 })

		assert.Equal(t, 0, w.vm(w.create(spec("web"))).UID)
	})

	t.Run("a name another vm answers to is a conflict", func(t *testing.T) {
		t.Parallel()

		w := newWorld(t, nil)

		w.create(spec("web"))

		_, err := w.engine.Create(w.ctx, spec("web"))
		assert.ErrorIs(t, err, vm.ErrConflict)
	})

	t.Run("an image naming nothing to run, for a vm naming nothing, is invalid", func(t *testing.T) {
		t.Parallel()

		w := newWorld(t, nil)
		w.images.Add("scratch:latest", vm.Image{Digest: "sha256:empty"})

		s := spec("nothing")
		s.Image = "scratch:latest"
		s.Command = nil

		_, err := w.engine.Create(w.ctx, s)
		assert.ErrorIs(t, err, vm.ErrInvalid)
	})

	t.Run("an image that cannot be made into a disk is refused as the image store refused it", func(t *testing.T) {
		t.Parallel()

		w := newWorld(t, nil)
		w.images.Fail(vm.ErrImage)

		_, err := w.engine.Create(w.ctx, spec("web"))
		assert.ErrorIs(t, err, vm.ErrImage)
	})
}

func TestEngine_Admission(t *testing.T) {
	t.Parallel()

	refused := func(t *testing.T, w *world, s vm.Spec) {
		t.Helper()

		_, err := w.engine.Create(w.ctx, s)
		assert.ErrorIs(t, err, vm.ErrCapacity)

		all, err := w.states.All(w.ctx)
		require.NoError(t, err)

		for _, v := range all {
			assert.NotEqual(t, s.Name, v.Spec.Name, "a vm refused is not written down")
		}
	}

	t.Run("memory, with each vm's vmm overhead, is held to the budget", func(t *testing.T) {
		t.Parallel()

		// room for two VMs of 256 MiB with their 64 MiB of overhead.
		w := newWorld(t, nil, func(c *Config) { c.MaxMemory = 640 << 20 })

		w.create(spec("first"))
		w.create(spec("second"))
		refused(t, w, spec("third"))
	})

	t.Run("a vm whose task ended gives its memory back, and takes it again when it boots", func(t *testing.T) {
		t.Parallel()

		w := newWorld(t, nil, func(c *Config) { c.MaxMemory = 640 << 20 })

		first := w.run(spec("first"))
		w.create(spec("second"))

		require.NoError(t, w.engine.Stop(w.ctx, first, 0))

		third := w.create(spec("third"))

		err := w.engine.Start(w.ctx, first)
		assert.ErrorIs(t, err, vm.ErrCapacity, "the room it gave back was taken")
		assert.Equal(t, vm.StateExited, w.vm(first).State)

		require.NoError(t, w.engine.Delete(w.ctx, third))
		require.NoError(t, w.engine.Start(w.ctx, first))
		assert.Equal(t, vm.StateRunning, w.vm(first).State)
	})

	t.Run("cpus are held to the host's times the overcommit", func(t *testing.T) {
		t.Parallel()

		w := newWorld(t, nil, func(c *Config) { c.HostCPUs, c.CPUOvercommit = 1, 2 })

		w.create(spec("first"))
		w.create(spec("second"))
		refused(t, w, spec("third"))
	})

	t.Run("the disk vmhost keeps free is not promised to anybody", func(t *testing.T) {
		t.Parallel()

		w := newWorld(t, nil)
		w.free = 1 << 30

		refused(t, w, spec("web"))

		readOnly := spec("read-only")
		readOnly.ReadOnly = true

		_, err := w.engine.Create(w.ctx, readOnly)
		assert.NoError(t, err, "a vm with no scratch disk takes no disk")
	})

	t.Run("scratch disks are held to the free disk times the overcommit", func(t *testing.T) {
		t.Parallel()

		// 2 GiB free, of which 1 GiB is kept, and 0.1 of it promised: one
		// scratch disk of 128 MiB.
		w := newWorld(t, nil, func(c *Config) { c.DiskOvercommit = 0.1 })
		w.free = 2 << 30

		w.create(spec("first"))
		refused(t, w, spec("second"))
	})

	t.Run("a vm is refused once every host user machines run as is taken", func(t *testing.T) {
		t.Parallel()

		w := newWorld(t, nil, func(c *Config) { c.UIDs = 1 })

		w.create(spec("first"))
		refused(t, w, spec("second"))
	})
}
