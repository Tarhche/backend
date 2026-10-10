package workload

import (
	"context"
	"log/slog"
	"maps"
	"slices"
	"sync"
	"testing"
	"time"

	"github.com/gofrs/uuid/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/khanzadimahdi/testproject/domain/workload/graph"
	containerKind "github.com/khanzadimahdi/testproject/domain/workload/kinds/container"
	networkKind "github.com/khanzadimahdi/testproject/domain/workload/kinds/network"
	stackKind "github.com/khanzadimahdi/testproject/domain/workload/kinds/stack"
	vmKind "github.com/khanzadimahdi/testproject/domain/workload/kinds/vm"
	volumeKind "github.com/khanzadimahdi/testproject/domain/workload/kinds/volume"
	"github.com/khanzadimahdi/testproject/domain/workload/resource"
	"github.com/khanzadimahdi/testproject/infrastructure/configs"
	messagingMock "github.com/khanzadimahdi/testproject/infrastructure/messaging/mock"
	resourcesMemory "github.com/khanzadimahdi/testproject/infrastructure/repository/memory/workload/resources"
)

// reading keeps records as the repository it wraps does, counting every
// read.
type reading struct {
	resource.Repository

	lock  sync.Mutex
	reads int
}

func (r *reading) read() {
	r.lock.Lock()
	defer r.lock.Unlock()

	r.reads++
}

func (r *reading) Reads() int {
	r.lock.Lock()
	defer r.lock.Unlock()

	return r.reads
}

func (r *reading) GetOne(ctx context.Context, kindName string, uuid string) (resource.Record, error) {
	r.read()

	return r.Repository.GetOne(ctx, kindName, uuid)
}

func (r *reading) GetAll(ctx context.Context, kindName string, filter resource.Filter, offset uint, limit uint) ([]resource.Record, uint, error) {
	r.read()

	return r.Repository.GetAll(ctx, kindName, filter, offset, limit)
}

// TestNewControlPlaneWorkload_graphs holds the control plane to keeping the
// graphs of stacks and of the containers nobody's stack owns, as a holder of
// leases of its own, and to not starting with settings it cannot go about.
func TestNewControlPlaneWorkload_graphs(t *testing.T) {
	t.Parallel()

	ctx := context.Background()

	t.Run("a graph is made of a vm, the building blocks what lives in it needs, and stacks, and a stack and a container nothing owns are roots of their own", func(t *testing.T) {
		t.Parallel()

		kinds := controlPlaneGraphKinds(nil, blockStrategies{}, nil)

		assert.Equal(t, []string{containerKind.Name, networkKind.Name, stackKind.Name, vmKind.Name, volumeKind.Name}, slices.Sorted(maps.Keys(kinds)))
		assert.Implements(t, (*graph.RootKind)(nil), kinds[containerKind.Name], "a container may be a root of its own")
		assert.Implements(t, (*graph.RootKind)(nil), kinds[stackKind.Name], "and so is a stack")

		assert.Equal(t, graph.Roots{stackKind.Name, containerKind.Name}, graphRoots())
	})

	t.Run("so a stack and a container are queued and reconciled, and what changed under one is followed up to it, and nothing else", func(t *testing.T) {
		t.Parallel()

		records := &reading{Repository: resourcesMemory.NewRepository()}

		stores := storesOf(resourcesMemory.NewRepository())
		stores.Resources = records

		workload, err := NewControlPlaneWorkload(configs.NewWorkloadControlPlane(), stores, nil, &messagingMock.Recorder{}, slog.New(slog.DiscardHandler))
		require.NoError(t, err)

		stack := graph.Ref{Kind: stackKind.Name, UUID: "s-uuid"}
		container := graph.Ref{Kind: containerKind.Name, UUID: "c-uuid"}

		workload.Graphs.Enqueue(stack)
		workload.Graphs.Enqueue(container)
		workload.Graphs.Enqueue(graph.Ref{Kind: vmKind.Name, UUID: "vm-uuid"})

		assert.ElementsMatch(t, []graph.Ref{stack, container}, workload.Graphs.Queued(), "a vm is no root")
		assert.NoError(t, workload.Graphs.Reconcile(ctx, stack), "a stack is one, and one that is gone is forgotten")
		assert.NoError(t, workload.Graphs.Reconcile(ctx, container), "and so is a container")

		require.NoError(t, workload.Graphs.Changed(ctx, graph.Ref{Kind: vmKind.Name, UUID: "vm-uuid"}))
		require.NoError(t, workload.Graphs.Resync(ctx))
		assert.NotZero(t, records.Reads(), "what changed is read")

		working, stop := context.WithCancel(ctx)

		worked := make(chan struct{})
		go func() {
			defer close(worked)

			workload.Graphs.Work(working)
		}()

		stop()

		select {
		case <-worked:
		case <-time.After(5 * time.Second):
			require.Fail(t, "its workers never stopped")
		}
	})

	t.Run("each control plane holds the leases of roots as a uuid of its own, made as it starts", func(t *testing.T) {
		t.Parallel()

		one := served(t).workload.Holder
		another := served(t).workload.Holder

		parsed, err := uuid.FromString(one)
		require.NoError(t, err)
		assert.Equal(t, byte(7), parsed.Version())

		assert.NotEqual(t, one, another)
	})

	for name, tt := range map[string]struct {
		change  func(c *configs.WorkloadControlPlane)
		setting string
	}{
		"no worker":                    {change: func(c *configs.WorkloadControlPlane) { c.GraphWorkers = 0 }, setting: "WORKLOAD_GRAPH_WORKERS"},
		"no node of a level at a time": {change: func(c *configs.WorkloadControlPlane) { c.GraphConcurrency = 0 }, setting: "WORKLOAD_GRAPH_CONCURRENCY"},
		"no resync":                    {change: func(c *configs.WorkloadControlPlane) { c.GraphResync = 0 }, setting: "WORKLOAD_GRAPH_RESYNC"},
		"a lease too short to be renewed a third at a time": {change: func(c *configs.WorkloadControlPlane) { c.GraphLease = time.Millisecond }, setting: "WORKLOAD_GRAPH_LEASE"},
	} {
		t.Run("not with "+name, func(t *testing.T) {
			t.Parallel()

			controlPlaneConfigs := configs.NewWorkloadControlPlane()
			tt.change(controlPlaneConfigs)

			workload, err := NewControlPlaneWorkload(controlPlaneConfigs, storesOf(resourcesMemory.NewRepository()), nil, &messagingMock.Recorder{}, slog.New(slog.DiscardHandler))
			assert.Nil(t, workload)
			assert.ErrorContains(t, err, tt.setting, "saying which setting")
		})
	}

	t.Run("nor with nowhere to keep how roots' plans went, or their leases", func(t *testing.T) {
		t.Parallel()

		for _, change := range []func(s *ControlPlaneStores){
			func(s *ControlPlaneStores) { s.Executions = nil },
			func(s *ControlPlaneStores) { s.Leases = nil },
		} {
			stores := storesOf(resourcesMemory.NewRepository())
			change(&stores)

			workload, err := NewControlPlaneWorkload(configs.NewWorkloadControlPlane(), stores, nil, &messagingMock.Recorder{}, slog.New(slog.DiscardHandler))
			assert.Nil(t, workload)
			assert.Error(t, err)
		}
	})
}
