package workload

import (
	"context"
	"encoding/json"
	"log/slog"
	"maps"
	"slices"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/gofrs/uuid/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/khanzadimahdi/testproject/application/workload/controlplane/kinds/blocks/blockstest"
	"github.com/khanzadimahdi/testproject/application/workload/controlplane/kinds/vm/vmtest"
	"github.com/khanzadimahdi/testproject/domain/workload/graph"
	"github.com/khanzadimahdi/testproject/domain/workload/kind"
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

// interloping keeps records as the repository it wraps does, but for doing
// what do says before the first write of a record that writes picks, and
// before writing it: what goes on while that write is yet to be made.
type interloping struct {
	resource.Repository

	writes func(r resource.Record) bool
	do     func(ctx context.Context)

	done atomic.Bool
}

func (i *interloping) Update(ctx context.Context, r resource.Record) (resource.Record, error) {
	if i.writes(r) && i.done.CompareAndSwap(false, true) {
		i.do(ctx)
	}

	return i.Repository.Update(ctx, r)
}

// TestNewControlPlaneWorkload_graphs holds the control plane to keeping the
// graphs of stacks and of the containers nobody's stack owns, as a holder of
// leases of its own, and to not starting with settings it cannot go about.
func TestNewControlPlaneWorkload_graphs(t *testing.T) {
	t.Parallel()

	ctx := context.Background()

	t.Run("a graph is made of a vm, the building blocks what lives in it needs, and stacks, and a stack and a container nothing owns are roots of their own", func(t *testing.T) {
		t.Parallel()

		kinds := controlPlaneGraphKinds(nil, blockStrategies{}, nil, time.Minute)

		assert.Equal(t, []string{containerKind.Name, networkKind.Name, stackKind.Name, vmKind.Name, volumeKind.Name}, slices.Sorted(maps.Keys(kinds)))
		assert.Implements(t, (*graph.RootKind)(nil), kinds[containerKind.Name], "a container may be a root of its own")
		assert.Implements(t, (*graph.RootKind)(nil), kinds[stackKind.Name], "and so is a stack")
		assert.Implements(t, (*graph.ResetKind)(nil), kinds[stackKind.Name], "which its vm's restore resets to what the restored disk holds of it")
		assert.NotImplements(t, (*graph.ResetKind)(nil), kinds[containerKind.Name], "while a container's own reset is its kind's, as its node finds it")

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

// TestNewControlPlaneWorkload_restored holds the control plane to deciding
// nothing of a stack its Docker VM's restore reset before that restore is
// taken onto the VM's record: what lives in the VM is reset first, so that a
// result heard again resets it again, and a run of the stack's graph in
// between reads the VM's record as the restore before left it, or none did.
func TestNewControlPlaneWorkload_restored(t *testing.T) {
	t.Parallel()

	ctx := context.Background()

	restoredAt := time.Now()
	before := restoredAt.Add(-2 * time.Hour)

	for name, earlier := range map[string]func(v *vmKind.VM){
		"its vm's first restore, which its vm's record says nothing of until it is taken onto it": func(*vmKind.VM) {},
		"a restore after one looked into long ago, which its vm's record says until then": func(v *vmKind.VM) {
			v.Status.RestoredAt = before
			v.Status.LookedAt = before.Add(time.Second)
		},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			stored := resourcesMemory.NewRepository()

			stack := graph.Ref{Kind: stackKind.Name, UUID: "s-1"}

			var workload *ControlPlaneWorkload

			// a run of the stack's graph, bounded, since one carrying the
			// stack out waits on commands no node is there to answer.
			run := func(ctx context.Context) error {
				bounded, cancel := context.WithTimeout(ctx, 10*time.Second)
				defer cancel()

				return workload.Graphs.Reconcile(bounded, stack)
			}

			// the stack's run in the window: once its vm's restore has
			// reset what lives in it, and before the restore is written onto
			// the vm's record.
			var (
				ran   error
				there bool
				reset bool
			)

			records := &interloping{
				Repository: stored,
				writes: func(r resource.Record) bool {
					return r.Kind == vmKind.Name && r.Metadata.UUID == "vm-1"
				},
				do: func(ctx context.Context) {
					ran = run(ctx)

					r, err := stored.GetOne(ctx, stackKind.Name, "s-1")
					there, reset = err == nil, r.Reset
				},
			}

			stores := storesOf(stored)
			stores.Resources = records

			produced := &messagingMock.Recorder{}

			var err error
			workload, err = NewControlPlaneWorkload(configs.NewWorkloadControlPlane(), stores, nil, produced, slog.New(slog.DiscardHandler))
			require.NoError(t, err)

			// vm-1, its restore asked as restore-1, and what lives in it: a
			// stack, running, and the container of its service.
			restoring := vmtest.In(vmtest.Docker("vm-1", "owner"), func(v *vmKind.VM) {
				v.Status.State = vmKind.Restoring
				earlier(v)
			})

			raw, err := kind.Encode(restoring)
			require.NoError(t, err)

			_, err = stored.Create(ctx, resource.Record{Raw: raw, Pending: &resource.Pending{Action: vmKind.ActionRestore, IDs: []string{"restore-1"}, SentAt: restoredAt}})
			require.NoError(t, err)

			raw, err = kind.Encode(stackKind.Stack{
				Kind:     stackKind.Name,
				Metadata: kind.Metadata{UUID: "s-1", Name: "shop", Slug: "shop", OwnerUUID: "owner", Refs: []kind.Reference{{Kind: vmKind.Name, UUID: "vm-1"}}, CreatedAt: before},
				Spec:     stackKind.Spec{VM: stackKind.VMChoice{UUID: "vm-1"}, Compose: "services:\n  web:\n    image: nginx:1.27\n"},
				Status:   stackKind.Status{Status: kind.Status{State: stackKind.Running, Expected: stackKind.Running, Since: before}},
			})
			require.NoError(t, err)

			_, err = stored.Create(ctx, resource.Record{Raw: raw})
			require.NoError(t, err)

			_, err = stored.Create(ctx, resource.Record{Raw: blockstest.AContainer("c-web", "vm-1", containerKind.Running, containerKind.Running, func(c *containerKind.Container) {
				c.Metadata.Name = "shop_web"
				c.Metadata.Owners = []kind.Reference{{Kind: stackKind.Name, UUID: "s-1"}}
				c.Metadata.Labels = map[string]string{stackKind.LabelStack: "s-1", stackKind.LabelService: "web"}
				c.Spec.Name = "shop_web"
			})})
			require.NoError(t, err)

			status, err := json.Marshal(vmKind.Status{Status: kind.Status{State: vmKind.Running}, RestoredAt: restoredAt})
			require.NoError(t, err)

			result, err := json.Marshal(kind.ResourceActedOn{ID: "restore-1", Kind: vmKind.Name, UUID: "vm-1", Action: vmKind.ActionRestore, Node: vmtest.Node, OK: true, Status: status, At: restoredAt})
			require.NoError(t, err)

			require.NoError(t, workload.Subscribers[kind.ResourceActedOnName].Handle(ctx, result))

			require.True(t, records.done.Load(), "the restore was written onto the vm's record")
			require.NoError(t, ran, "the stack's run in between")
			assert.True(t, there, "which does not forget it")
			assert.True(t, reset, "nor carries it out, its restored vm yet to be looked into")

			v, err := stored.GetOne(ctx, vmKind.Name, "vm-1")
			require.NoError(t, err)

			taken, err := kind.Decode[vmKind.Spec, vmKind.Status](v.Raw)
			require.NoError(t, err)
			assert.Equal(t, vmKind.Running, taken.Status.State)
			assert.True(t, taken.Status.RestoredAt.Equal(restoredAt), "the restore is taken onto it once what lives in it is reset")

			require.NoError(t, run(ctx))

			s, err := stored.GetOne(ctx, stackKind.Name, "s-1")
			require.NoError(t, err)
			assert.True(t, s.Reset, "and the stack waits for its restored vm to be looked into")

			assert.Empty(t, produced.Subjects(), "nothing of it is asked of its node")
		})
	}
}
