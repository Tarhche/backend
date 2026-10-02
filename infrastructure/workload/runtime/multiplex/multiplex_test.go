package multiplex

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
	"net"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/khanzadimahdi/testproject/domain"
	"github.com/khanzadimahdi/testproject/domain/workload/driver"
	"github.com/khanzadimahdi/testproject/domain/workload/node"
	"github.com/khanzadimahdi/testproject/domain/workload/port"
	"github.com/khanzadimahdi/testproject/domain/workload/runtime"
	"github.com/khanzadimahdi/testproject/domain/workload/task"
	driverMock "github.com/khanzadimahdi/testproject/infrastructure/repository/mocks/workload/driver"
	runtimeMock "github.com/khanzadimahdi/testproject/infrastructure/repository/mocks/workload/runtime"
)

// vmID is what vmhost calls a VM.
const vmID = "0123456789abcdef"

// set is a node's drivers, found by their class as the registry finds them.
type set []driver.Driver

func (s set) For(class runtime.Class) (driver.Driver, error) {
	for _, d := range s {
		if d.Class() == class.OrSysbox() {
			return d, nil
		}
	}

	return nil, driver.ErrUnknownClass
}

func (s set) All() []driver.Driver {
	return s
}

// fixture is a node with sysbox's containers and firecracker's VMs, each
// class's runs, networks and stats standing in for its driver's.
type fixture struct {
	sysboxTasks      runtimeMock.MockRuntime
	sysboxNetworks   runtimeMock.MockNetworkManager
	sysboxNode       runtimeMock.MockNodeManager
	firecrackerTasks runtimeMock.MockDialingRuntime
	firecrackerNets  runtimeMock.MockNetworkManager
	firecrackerNode  runtimeMock.MockNodeManager

	sysbox      *driverMock.MockDriver
	firecracker *driverMock.MockDriver

	multiplexer *Multiplexer
}

func newFixture() *fixture {
	f := &fixture{}

	f.sysbox = classOf(runtime.Sysbox, driver.KindContainer, &f.sysboxTasks, &f.sysboxNetworks, &f.sysboxNode)
	f.firecracker = classOf(runtime.Firecracker, driver.KindMicroVM, &f.firecrackerTasks, &f.firecrackerNets, &f.firecrackerNode)

	f.multiplexer = New(set{f.sysbox, f.firecracker}, slog.New(slog.DiscardHandler))

	return f
}

func classOf(class runtime.Class, kind driver.Kind, tasks task.Runtime, networks *runtimeMock.MockNetworkManager, manager *runtimeMock.MockNodeManager) *driverMock.MockDriver {
	d := &driverMock.MockDriver{}
	d.On("Class").Return(class).Maybe()
	d.On("Kind").Return(kind).Maybe()
	d.On("Tasks").Return(tasks).Maybe()
	d.On("Networks").Return(networks).Maybe()
	d.On("Node").Return(manager).Maybe()
	d.On("Offer", mock.Anything).Return(runtime.Offer{Class: class, Driver: kind.String(), Healthy: true}).Maybe()

	return d
}

func TestMultiplexer_listing(t *testing.T) {
	t.Parallel()

	ctx := context.Background()

	t.Run("every class's runs, each named by its class", func(t *testing.T) {
		t.Parallel()

		f := newFixture()

		f.sysboxTasks.On("OnNode", mock.Anything, "node-1").Return([]task.Execution{{ID: "4f2c9d0b7a1e", TaskUUID: "task-1"}}, nil)
		f.firecrackerTasks.On("OnNode", mock.Anything, "node-1").Return([]task.Execution{{ID: vmID, TaskUUID: "task-2"}}, nil)

		held, err := f.multiplexer.OnNode(ctx, "node-1")
		require.NoError(t, err)

		// a sysbox run keeps the name docker gave it, as every run from before
		// there were classes has.
		assert.Equal(t, []task.Execution{
			{ID: "4f2c9d0b7a1e", TaskUUID: "task-1", Runtime: runtime.Sysbox},
			{ID: "firecracker:" + vmID, TaskUUID: "task-2", Runtime: runtime.Firecracker},
		}, held)
	})

	t.Run("a task's runs, and a slug's, are asked of every class", func(t *testing.T) {
		t.Parallel()

		f := newFixture()

		f.sysboxTasks.On("Of", mock.Anything, "task-2").Return([]task.Execution{}, nil)
		f.firecrackerTasks.On("Of", mock.Anything, "task-2").Return([]task.Execution{{ID: vmID}}, nil)
		f.sysboxTasks.On("BySlug", mock.Anything, "web-abcde").Return([]task.Execution{{ID: "4f2c9d0b7a1e"}}, nil)
		f.firecrackerTasks.On("BySlug", mock.Anything, "web-abcde").Return([]task.Execution{}, nil)

		runs, err := f.multiplexer.Of(ctx, "task-2")
		require.NoError(t, err)
		assert.Equal(t, []task.Execution{{ID: "firecracker:" + vmID, Runtime: runtime.Firecracker}}, runs)

		runs, err = f.multiplexer.BySlug(ctx, "web-abcde")
		require.NoError(t, err)
		assert.Equal(t, []task.Execution{{ID: "4f2c9d0b7a1e", Runtime: runtime.Sysbox}}, runs)
	})

	t.Run("a class that cannot be asked is left out, and its offer says why", func(t *testing.T) {
		t.Parallel()

		f := newFixture()

		f.sysboxTasks.On("OnNode", mock.Anything, "node-1").Return([]task.Execution{{ID: "4f2c9d0b7a1e"}}, nil)
		f.firecrackerTasks.On("OnNode", mock.Anything, "node-1").
			Return([]task.Execution(nil), errors.New("vmhost cannot be reached")).Once()

		held, err := f.multiplexer.OnNode(ctx, "node-1")
		require.NoError(t, err)
		assert.Equal(t, []string{"4f2c9d0b7a1e"}, idsOf(held))

		offers := offersOf(f.multiplexer.Drivers())
		assert.True(t, offers[0].Healthy)
		assert.False(t, offers[1].Healthy)
		assert.Contains(t, offers[1].Reason, "vmhost cannot be reached")

		// and once it can be asked again, it is offered as its driver offers it.
		f.firecrackerTasks.On("OnNode", mock.Anything, "node-1").Return([]task.Execution{{ID: vmID}}, nil).Once()

		held, err = f.multiplexer.OnNode(ctx, "node-1")
		require.NoError(t, err)
		assert.Equal(t, []string{"4f2c9d0b7a1e", "firecracker:" + vmID}, idsOf(held))

		offers = offersOf(f.multiplexer.Drivers())
		assert.True(t, offers[1].Healthy)
		assert.Empty(t, offers[1].Reason)
	})

	t.Run("only when every class fails does the question fail", func(t *testing.T) {
		t.Parallel()

		f := newFixture()

		dockerDown := errors.New("docker cannot be reached")
		vmhostDown := errors.New("vmhost cannot be reached")

		f.sysboxTasks.On("Of", mock.Anything, "task-1").Return([]task.Execution(nil), dockerDown)
		f.firecrackerTasks.On("Of", mock.Anything, "task-1").Return([]task.Execution(nil), vmhostDown)

		_, err := f.multiplexer.Of(ctx, "task-1")

		assert.ErrorIs(t, err, dockerDown)
		assert.ErrorIs(t, err, vmhostDown)
	})

	t.Run("one class alone fails as it always did", func(t *testing.T) {
		t.Parallel()

		var tasks runtimeMock.MockRuntime

		dockerDown := errors.New("docker cannot be reached")
		tasks.On("OnNode", mock.Anything, "node-1").Return([]task.Execution(nil), dockerDown)

		multiplexer := New(set{classOf(runtime.Sysbox, driver.KindContainer, &tasks, nil, nil)}, slog.New(slog.DiscardHandler))

		_, err := multiplexer.OnNode(ctx, "node-1")
		assert.ErrorIs(t, err, dockerDown)
	})
}

func TestMultiplexer_routing(t *testing.T) {
	t.Parallel()

	ctx := context.Background()

	t.Run("a run's commands go to the class holding it, by the name it has there", func(t *testing.T) {
		t.Parallel()

		f := newFixture()

		f.firecrackerTasks.On("Start", mock.Anything, vmID).Return(nil).Once()
		f.firecrackerTasks.On("Stop", mock.Anything, vmID).Return(nil).Once()
		f.firecrackerTasks.On("Restart", mock.Anything, vmID).Return(nil).Once()
		f.firecrackerTasks.On("Kill", mock.Anything, vmID).Return(nil).Once()
		f.firecrackerTasks.On("Delete", mock.Anything, vmID).Return(domain.ErrNotExists).Once()
		f.firecrackerTasks.On("Stats", mock.Anything, vmID).Return(task.Stats{PIDs: 3}, nil).Once()
		f.firecrackerTasks.On("Logs", mock.Anything, vmID, mock.Anything).Return(nil).Once()
		f.firecrackerTasks.On("StreamLogs", mock.Anything, vmID, mock.Anything, mock.Anything).Return(nil).Once()
		f.firecrackerTasks.On("Exec", mock.Anything, vmID, task.ExecOptions{TTY: true}).Return(nil, nil).Once()
		defer f.firecrackerTasks.AssertExpectations(t)

		id := "firecracker:" + vmID

		require.NoError(t, f.multiplexer.Start(ctx, id))
		require.NoError(t, f.multiplexer.Stop(ctx, id))
		require.NoError(t, f.multiplexer.Restart(ctx, id))
		require.NoError(t, f.multiplexer.Kill(ctx, id))

		// what the class says comes back as it said it.
		assert.ErrorIs(t, f.multiplexer.Delete(ctx, id), domain.ErrNotExists)

		stats, err := f.multiplexer.Stats(ctx, id)
		require.NoError(t, err)
		assert.Equal(t, uint64(3), stats.PIDs)

		require.NoError(t, f.multiplexer.Logs(ctx, id, &bytes.Buffer{}))
		require.NoError(t, f.multiplexer.StreamLogs(ctx, id, time.Time{}, func(task.LogLine) error { return nil }))

		_, err = f.multiplexer.Exec(ctx, id, task.ExecOptions{TTY: true})
		require.NoError(t, err)

		f.sysboxTasks.AssertNotCalled(t, "Start", mock.Anything, mock.Anything)
	})

	t.Run("a bare ID is sysbox's, as every run's was before there were classes", func(t *testing.T) {
		t.Parallel()

		f := newFixture()

		f.sysboxTasks.On("Stop", mock.Anything, "4f2c9d0b7a1e").Return(nil).Once()
		defer f.sysboxTasks.AssertExpectations(t)

		require.NoError(t, f.multiplexer.Stop(ctx, "4f2c9d0b7a1e"))
	})

	t.Run("an inspected run is named by its class", func(t *testing.T) {
		t.Parallel()

		f := newFixture()

		f.firecrackerTasks.On("Inspect", mock.Anything, vmID).Return(task.Execution{ID: vmID, ExitCode: 137}, nil)

		inspected, err := f.multiplexer.Inspect(ctx, "firecracker:"+vmID)
		require.NoError(t, err)

		assert.Equal(t, task.Execution{ID: "firecracker:" + vmID, Runtime: runtime.Firecracker, ExitCode: 137}, inspected)
	})

	t.Run("a run is made by the class it names, and sysbox when it names none", func(t *testing.T) {
		t.Parallel()

		f := newFixture()

		f.firecrackerTasks.On("Create", mock.Anything, mock.Anything).Return(vmID, nil).Once()
		f.sysboxTasks.On("Create", mock.Anything, mock.Anything).Return("4f2c9d0b7a1e", nil).Once()

		id, err := f.multiplexer.Create(ctx, &task.Execution{Runtime: runtime.Firecracker})
		require.NoError(t, err)
		assert.Equal(t, "firecracker:"+vmID, id)

		id, err = f.multiplexer.Create(ctx, &task.Execution{})
		require.NoError(t, err)
		assert.Equal(t, "4f2c9d0b7a1e", id)
	})

	t.Run("a class this node does not offer is one it does not know", func(t *testing.T) {
		t.Parallel()

		f := newFixture()

		assert.ErrorIs(t, f.multiplexer.Start(ctx, "gvisor:4f2c9d0b7a1e"), driver.ErrUnknownClass)

		_, err := f.multiplexer.Create(ctx, &task.Execution{Runtime: "gvisor"})
		assert.ErrorIs(t, err, driver.ErrUnknownClass)
	})

	t.Run("making an image or a network ready is asked of the class's own driver", func(t *testing.T) {
		t.Parallel()

		f := newFixture()

		assert.ErrorIs(t, f.multiplexer.EnsureImage(ctx, "busybox"), ErrClassRequired)
		assert.ErrorIs(t, f.multiplexer.EnsureIsolatedNetwork(ctx), ErrClassRequired)
		assert.ErrorIs(t, f.multiplexer.EnsureStackNetwork(ctx, "shop-abcde"), ErrClassRequired)
	})
}

func TestMultiplexer_DialContext(t *testing.T) {
	t.Parallel()

	ctx := context.Background()

	t.Run("a class that dials its runs is asked to", func(t *testing.T) {
		t.Parallel()

		f := newFixture()

		client, server := net.Pipe()
		defer client.Close()
		defer server.Close()

		f.firecrackerTasks.On("DialContext", mock.Anything, vmID, port.Port(80)).Return(client, nil).Once()

		conn, err := f.multiplexer.DialContext(ctx, "firecracker:"+vmID, 80)
		require.NoError(t, err)
		assert.Same(t, client, conn)
	})

	t.Run("one that cannot says so, so its runs are reached another way", func(t *testing.T) {
		t.Parallel()

		f := newFixture()

		_, err := f.multiplexer.DialContext(ctx, "4f2c9d0b7a1e", 80)
		assert.ErrorIs(t, err, task.ErrNotSupported)
	})
}

func TestMultiplexer_networks(t *testing.T) {
	t.Parallel()

	ctx := context.Background()

	t.Run("a stack whose class is not known has its network dropped by every class", func(t *testing.T) {
		t.Parallel()

		f := newFixture()

		f.sysboxNetworks.On("RemoveStackNetwork", mock.Anything, "shop-abcde").Return(nil).Once()

		// one that has no such network has nothing to drop.
		f.firecrackerNets.On("RemoveStackNetwork", mock.Anything, "shop-abcde").Return(domain.ErrNotExists).Once()

		require.NoError(t, f.multiplexer.RemoveStackNetwork(ctx, "shop-abcde"))

		f.sysboxNetworks.AssertExpectations(t)
		f.firecrackerNets.AssertExpectations(t)
	})

	t.Run("a network that will not go is said to", func(t *testing.T) {
		t.Parallel()

		f := newFixture()

		stuck := errors.New("the network still holds tasks")

		f.sysboxNetworks.On("RemoveStackNetwork", mock.Anything, "shop-abcde").Return(stuck)
		f.firecrackerNets.On("RemoveStackNetwork", mock.Anything, "shop-abcde").Return(nil)

		assert.ErrorIs(t, f.multiplexer.RemoveStackNetwork(ctx, "shop-abcde"), stuck)
	})
}

func TestMultiplexer_Node(t *testing.T) {
	t.Parallel()

	ctx := context.Background()

	t.Run("what the node's runs use is every class's added up", func(t *testing.T) {
		t.Parallel()

		f := newFixture()

		f.sysboxNode.On("Stats", mock.Anything, "node-1").Return(node.Stats{PIDs: 2, MemoryUsage: 50, MemoryLimit: 100}, nil)
		f.firecrackerNode.On("Stats", mock.Anything, "node-1").Return(node.Stats{PIDs: 1, MemoryUsage: 150, MemoryLimit: 300}, nil)

		stats, err := f.multiplexer.Node().Stats(ctx, "node-1")
		require.NoError(t, err)

		assert.Equal(t, node.Stats{PIDs: 3, MemoryUsage: 200, MemoryLimit: 400, MemoryPercent: 50}, stats)
	})

	t.Run("a class that cannot say is left out", func(t *testing.T) {
		t.Parallel()

		f := newFixture()

		f.sysboxNode.On("Stats", mock.Anything, "node-1").Return(node.Stats{PIDs: 2}, nil)
		f.firecrackerNode.On("Stats", mock.Anything, "node-1").Return(node.Stats{}, errors.New("vmhost cannot be reached"))

		stats, err := f.multiplexer.Node().Stats(ctx, "node-1")
		require.NoError(t, err)

		assert.Equal(t, uint64(2), stats.PIDs)
	})

	t.Run("a class whose runs' use cannot be read is offered unhealthy until it can", func(t *testing.T) {
		t.Parallel()

		f := newFixture()

		f.sysboxNode.On("Stats", mock.Anything, "node-1").Return(node.Stats{PIDs: 2}, nil)
		f.firecrackerNode.On("Stats", mock.Anything, "node-1").Return(node.Stats{}, errors.New("vmhost cannot be reached")).Once()

		_, err := f.multiplexer.Node().Stats(ctx, "node-1")
		require.NoError(t, err)

		offers := offersOf(f.multiplexer.Drivers())
		assert.True(t, offers[0].Healthy)
		assert.False(t, offers[1].Healthy)
		assert.Equal(t, "what its runs use cannot be read: vmhost cannot be reached", offers[1].Reason)

		f.firecrackerNode.On("Stats", mock.Anything, "node-1").Return(node.Stats{PIDs: 1}, nil).Once()

		_, err = f.multiplexer.Node().Stats(ctx, "node-1")
		require.NoError(t, err)

		assert.True(t, offersOf(f.multiplexer.Drivers())[1].Healthy)
	})

	t.Run("a class answering one question is not taken for answering the other", func(t *testing.T) {
		t.Parallel()

		f := newFixture()

		f.sysboxTasks.On("OnNode", mock.Anything, "node-1").Return([]task.Execution{}, nil)
		f.firecrackerTasks.On("OnNode", mock.Anything, "node-1").Return([]task.Execution(nil), errors.New("vmhost cannot be reached"))
		f.sysboxNode.On("Stats", mock.Anything, "node-1").Return(node.Stats{}, nil)
		f.firecrackerNode.On("Stats", mock.Anything, "node-1").Return(node.Stats{}, nil)

		_, err := f.multiplexer.OnNode(ctx, "node-1")
		require.NoError(t, err)

		_, err = f.multiplexer.Node().Stats(ctx, "node-1")
		require.NoError(t, err)

		offer := offersOf(f.multiplexer.Drivers())[1]
		assert.False(t, offer.Healthy)
		assert.Equal(t, "its runs cannot be listed: vmhost cannot be reached", offer.Reason)
	})

	t.Run("a node none of whose classes can say has nothing to report", func(t *testing.T) {
		t.Parallel()

		f := newFixture()

		f.sysboxNode.On("Stats", mock.Anything, "node-1").Return(node.Stats{}, errors.New("docker cannot be reached"))
		f.firecrackerNode.On("Stats", mock.Anything, "node-1").Return(node.Stats{}, errors.New("vmhost cannot be reached"))

		_, err := f.multiplexer.Node().Stats(ctx, "node-1")
		assert.Error(t, err)
	})
}

func TestMultiplexer_Drivers(t *testing.T) {
	t.Parallel()

	t.Run("are the node's own drivers", func(t *testing.T) {
		t.Parallel()

		f := newFixture()

		found, err := f.multiplexer.Drivers().For(runtime.Firecracker)
		require.NoError(t, err)

		assert.Equal(t, runtime.Firecracker, found.Class())
		assert.Same(t, &f.firecrackerTasks, found.Tasks())

		_, err = f.multiplexer.Drivers().For("gvisor")
		assert.ErrorIs(t, err, driver.ErrUnknownClass)

		assert.Len(t, f.multiplexer.Drivers().All(), 2)
	})
}

func idsOf(runs []task.Execution) []string {
	ids := make([]string, len(runs))
	for i := range runs {
		ids[i] = runs[i].ID
	}

	return ids
}

func offersOf(drivers driver.Set) []runtime.Offer {
	all := drivers.All()

	offers := make([]runtime.Offer, len(all))
	for i, d := range all {
		offers[i] = d.Offer(context.Background())
	}

	return offers
}
