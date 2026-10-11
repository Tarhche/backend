package workload

import (
	"context"
	"encoding/json"
	"errors"
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
	"github.com/khanzadimahdi/testproject/application/workload/controlplane/kinds/observe"
	kindsReconcileResources "github.com/khanzadimahdi/testproject/application/workload/controlplane/kinds/reconcileResources"
	"github.com/khanzadimahdi/testproject/application/workload/controlplane/kinds/vm/vmtest"
	"github.com/khanzadimahdi/testproject/domain"
	"github.com/khanzadimahdi/testproject/domain/workload/graph"
	"github.com/khanzadimahdi/testproject/domain/workload/kind"
	blockKinds "github.com/khanzadimahdi/testproject/domain/workload/kinds/blocks"
	containerKind "github.com/khanzadimahdi/testproject/domain/workload/kinds/container"
	networkKind "github.com/khanzadimahdi/testproject/domain/workload/kinds/network"
	stackKind "github.com/khanzadimahdi/testproject/domain/workload/kinds/stack"
	vmKind "github.com/khanzadimahdi/testproject/domain/workload/kinds/vm"
	volumeKind "github.com/khanzadimahdi/testproject/domain/workload/kinds/volume"
	"github.com/khanzadimahdi/testproject/domain/workload/resource"
	"github.com/khanzadimahdi/testproject/infrastructure/configs"
	messagingMock "github.com/khanzadimahdi/testproject/infrastructure/messaging/mock"
	nodesMemory "github.com/khanzadimahdi/testproject/infrastructure/repository/memory/workload/nodes"
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
// between reads the VM's record as the restore before left it, or none did;
// whether the record says it is restoring still, or, its node gone quiet in
// between, failed as lost, it waits on the restore no more, which is taken
// onto it all the same.
func TestNewControlPlaneWorkload_restored(t *testing.T) {
	t.Parallel()

	ctx := context.Background()

	restoredAt := time.Now()
	before := restoredAt.Add(-2 * time.Hour)

	for name, tt := range map[string]struct {
		// earlier is what the vm's record says of a restore before.
		earlier func(v *vmKind.VM)

		// window is what is written of the vm's record once what lives in
		// it is reset, before the stack's run.
		window func(t *testing.T, ctx context.Context, stored *resourcesMemory.Repository)

		// state is what the vm's record says it is doing in the end.
		state kind.State
	}{
		"its vm's first restore, which its vm's record says nothing of until it is taken onto it": {
			earlier: func(*vmKind.VM) {},
			state:   vmKind.Running,
		},
		"a restore after one looked into long ago, which its vm's record says until then": {
			earlier: lookedLongAgo(before),
			state:   vmKind.Running,
		},
		"its vm's first restore, its record failed as lost meanwhile, its node gone quiet, waiting on the restore no more": {
			earlier: func(*vmKind.VM) {},
			window:  lost,
			state:   kind.Failed,
		},
		"a restore after one looked into long ago, its record failed as lost meanwhile": {
			earlier: lookedLongAgo(before),
			window:  lost,
			state:   kind.Failed,
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
					if tt.window != nil {
						tt.window(t, ctx, stored)
					}

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
				tt.earlier(v)
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

			// its node said so a moment after it carried it out.
			status, err := json.Marshal(vmKind.Status{Status: kind.Status{State: vmKind.Running}, RestoredAt: restoredAt})
			require.NoError(t, err)

			result, err := json.Marshal(kind.ResourceActedOn{ID: "restore-1", Kind: vmKind.Name, UUID: "vm-1", Action: vmKind.ActionRestore, Node: vmtest.Node, OK: true, Status: status, At: restoredAt.Add(time.Second)})
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
			assert.Equal(t, tt.state, taken.Status.State)
			assert.True(t, taken.Status.RestoredAt.Equal(restoredAt), "the restore is taken onto it once what lives in it is reset")

			require.NoError(t, run(ctx))

			s, err := stored.GetOne(ctx, stackKind.Name, "s-1")
			require.NoError(t, err)
			assert.True(t, s.Reset, "and the stack waits for its restored vm to be looked into")
			assert.True(t, s.ResetAt.Equal(restoredAt.Truncate(time.Millisecond)), "reset for that restore: %s", s.ResetAt)

			assert.Empty(t, produced.Subjects(), "nothing of it is asked of its node")
		})
	}
}

// lookedLongAgo has a vm's record say its disk was restored at a moment, and
// looked into a second after.
func lookedLongAgo(at time.Time) func(v *vmKind.VM) {
	return func(v *vmKind.VM) {
		v.Status.RestoredAt = at
		v.Status.LookedAt = at.Add(time.Second)
	}
}

// lost has the record of vm-1 failed as its node's going quiet fails it,
// waiting on no command any more, as the reconcile loop writes it.
func lost(t *testing.T, ctx context.Context, stored *resourcesMemory.Repository) {
	t.Helper()

	r, err := stored.GetOne(ctx, vmKind.Name, "vm-1")
	require.NoError(t, err)

	require.NoError(t, observe.Fail(&r, kindsReconcileResources.ReasonNodeLost, time.Now()))

	// its node went on beating meanwhile, and was heard of long after the
	// look a restore before had.
	common, err := r.Common()
	require.NoError(t, err)

	common.ObservedAt = time.Now()
	require.NoError(t, r.SetCommon(common))

	_, err = stored.Update(ctx, r)
	require.NoError(t, err)
}

// looking keeps records as the repository it wraps does, but for doing what
// do says, once armed, before the first read of the record of the kind and
// the uuid it names: what goes on while that read is yet to be made.
type looking struct {
	resource.Repository

	kind string
	uuid string
	do   func(ctx context.Context)

	armed atomic.Bool
}

func (l *looking) GetOne(ctx context.Context, kindName string, uuid string) (resource.Record, error) {
	if kindName == l.kind && uuid == l.uuid && l.armed.CompareAndSwap(true, false) {
		l.do(ctx)
	}

	return l.Repository.GetOne(ctx, kindName, uuid)
}

// TestNewControlPlaneWorkload_vmDeletedUnderARun holds a run of the graph of a
// container nothing owns, planned before its VM's delete was carried out, to
// making nothing once the delete's cascade kept the container, to make the VM
// again as it was, and took the VM's record away: the VM the run's plan names
// by its uuid alone would be made named by nothing else, under a new slug,
// with what any Docker VM is given. The run after plans it again.
func TestNewControlPlaneWorkload_vmDeletedUnderARun(t *testing.T) {
	t.Parallel()

	ctx := context.Background()

	stored := resourcesMemory.NewRepository()

	var workload *ControlPlaneWorkload

	// the vm's delete carried out, its result heard, as the run that
	// planned the container's graph before looks for the vm.
	deleted, err := json.Marshal(kind.ResourceActedOn{ID: "delete-1", Kind: vmKind.Name, UUID: "vm-1", Action: vmKind.ActionDelete, Node: vmtest.Node, OK: true, At: time.Now()})
	require.NoError(t, err)

	records := &looking{Repository: stored, kind: vmKind.Name, uuid: "vm-1", do: func(ctx context.Context) {
		require.NoError(t, workload.Subscribers[kind.ResourceActedOnName].Handle(ctx, deleted))
	}}

	stores := storesOf(stored)
	stores.Resources = records
	stores.Nodes = nodesMemory.NewRepository(vmtest.Alive(vmtest.Node))

	produced := &messagingMock.Recorder{}

	workload, err = NewControlPlaneWorkload(configs.NewWorkloadControlPlane(), stores, nil, produced, slog.New(slog.DiscardHandler))
	require.NoError(t, err)

	// vm-1, named builds, being deleted, and a container nothing owns in it,
	// which names it by its uuid alone.
	deleting := vmtest.In(vmtest.Docker("vm-1", "owner"), func(v *vmKind.VM) {
		v.Metadata.Name = "builds"
		v.Metadata.Slug = "builds-abcde"
		v.Status.State = vmKind.Deleting
		v.Status.Expected = kind.Deleted
	})

	raw, err := kind.Encode(deleting)
	require.NoError(t, err)

	_, err = stored.Create(ctx, resource.Record{Raw: raw, Pending: &resource.Pending{Action: vmKind.ActionDelete, IDs: []string{"delete-1"}, SentAt: time.Now()}})
	require.NoError(t, err)

	_, err = stored.Create(ctx, resource.Record{Raw: blockstest.AContainer("c-1", "vm-1", containerKind.Running, containerKind.Running, func(c *containerKind.Container) {
		c.Metadata.Generation = 1
	})})
	require.NoError(t, err)

	records.armed.Store(true)

	bounded, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	ran := workload.Graphs.Reconcile(bounded, graph.Ref{Kind: containerKind.Name, UUID: "c-1"})
	require.False(t, records.armed.Load(), "the vm's delete was carried out as the run looked for the vm")

	made, err := stored.GetOne(ctx, vmKind.Name, "vm-1")
	assert.ErrorIs(t, err, domain.ErrNotExists, "no vm is made of the plan the run made before the container was kept: %s, %s", made.Metadata.Name, made.Metadata.Slug)
	assert.Empty(t, produced.Subjects(), "nor asked of a node")
	require.NoError(t, ran, "the run waits, which is no failure")

	c, err := stored.GetOne(ctx, containerKind.Name, "c-1")
	require.NoError(t, err)

	kept, err := kind.Decode[containerKind.Spec, containerKind.Status](c.Raw)
	require.NoError(t, err)
	assert.Equal(t, "builds", kept.Spec.VM.Name, "the container keeps what it makes its vm again with")
	assert.Equal(t, int64(2), kept.Metadata.Generation, "a generation later")

	assert.Contains(t, workload.Graphs.Queued(), graph.Ref{Kind: containerKind.Name, UUID: "c-1"}, "and it is queued, to be planned again")
}

// TestNewControlPlaneWorkload_restoredWhileItsDockerdComesUp holds the
// control plane to keeping the records of what a Docker VM's restore reset
// for as long as the VM's dockerd takes to come up, and to the first look into
// the restored disk deciding each: found there by its uuid, whatever became
// of that look's own heartbeat of it, or forgotten once its node goes on
// without a word of it since the look. A stack is then held by what the disk
// holds of it under the uuids it had, which nothing has to take in again.
func TestNewControlPlaneWorkload_restoredWhileItsDockerdComesUp(t *testing.T) {
	t.Parallel()

	for name, late := range map[string]bool{
		"the look's own heartbeat of its container lost, and the beat after it heard":                                                       false,
		"the look said by its vm's bridge network, and the look's heartbeat of its container heard after its vm's, the stack's run between": true,
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			ctx := context.Background()

			stored := resourcesMemory.NewRepository()

			stores := storesOf(stored)
			stores.Nodes = nodesMemory.NewRepository(vmtest.Alive(vmtest.Node))

			workload, err := NewControlPlaneWorkload(configs.NewWorkloadControlPlane(), stores, nil, &messagingMock.Recorder{}, slog.New(slog.DiscardHandler))
			require.NoError(t, err)

			// vm-1, its disk restored ten minutes ago, its dockerd up three
			// minutes after, which is when it is first looked into.
			restoredAt := time.Now().Add(-10 * time.Minute)
			first := restoredAt.Add(3 * time.Minute)
			resetAt := restoredAt.Truncate(time.Millisecond)

			// it runs, and its node says so beat after beat.
			v := vmtest.In(vmtest.Docker("vm-1", "owner"), func(v *vmKind.VM) {
				v.Status.RestoredAt = restoredAt
				v.Status.ObservedAt = time.Now()
			})
			raw, err := kind.Encode(v)
			require.NoError(t, err)

			_, err = stored.Create(ctx, resource.Record{Raw: raw})
			require.NoError(t, err)

			// the stack and its web server, both reset for that restore.
			raw, err = kind.Encode(stackKind.Stack{
				Kind:     stackKind.Name,
				Metadata: kind.Metadata{UUID: "s-1", Name: "shop", Slug: "shop", OwnerUUID: "owner", Refs: []kind.Reference{{Kind: vmKind.Name, UUID: "vm-1"}}, CreatedAt: restoredAt.Add(-time.Hour)},
				Spec:     stackKind.Spec{VM: stackKind.VMChoice{UUID: "vm-1"}, Compose: "services:\n  web:\n    image: nginx:1.27\n"},
				Status:   stackKind.Status{Status: kind.Status{State: stackKind.Running, Expected: stackKind.Running, Since: restoredAt.Add(-time.Hour)}},
			})
			require.NoError(t, err)

			_, err = stored.Create(ctx, resource.Record{Raw: raw, Reset: true, ResetAt: resetAt})
			require.NoError(t, err)

			_, err = stored.Create(ctx, resource.Record{Raw: blockstest.AContainer("c-web", "vm-1", containerKind.Running, containerKind.Running, func(c *containerKind.Container) {
				c.Metadata.Name = "shop_web"
				c.Metadata.Owners = []kind.Reference{{Kind: stackKind.Name, UUID: "s-1"}}
				c.Metadata.Labels = map[string]string{stackKind.LabelStack: "s-1", stackKind.LabelService: "web"}
				c.Spec.Name = "shop_web"
				c.Status.ObservedAt = restoredAt
			}), Reset: true, ResetAt: resetAt})
			require.NoError(t, err)

			beat := func(at time.Time, observed kind.Observation) {
				t.Helper()

				data, err := json.Marshal(kind.Heartbeat{Node: vmtest.Node, At: at, Observed: observed})
				require.NoError(t, err)
				require.NoError(t, workload.Subscribers[kind.HeartbeatName(observed.Kind)].Handle(ctx, data))
			}

			// the web server as the restored disk holds it, under its uuid.
			spec, err := containerKind.SpecLabel(containerKind.Spec{Name: "shop_web", Image: "nginx:1.27"})
			require.NoError(t, err)

			labels := blockKinds.Labels(containerKind.Name, "c-web")
			labels[containerKind.LabelSpec] = spec
			labels[stackKind.LabelStack] = "s-1"
			labels[stackKind.LabelService] = "web"

			status, err := json.Marshal(containerKind.Status{Status: kind.Status{State: containerKind.Running}, Docker: &containerKind.Docker{ID: "d-web", Name: "shop_web", State: "running", Labels: labels}})
			require.NoError(t, err)

			web := kind.Observation{Kind: containerKind.Name, UUID: "c-web", Owners: []kind.Reference{{Kind: vmKind.Name, UUID: "vm-1"}}, Status: status}

			bridgeStatus, err := json.Marshal(map[string]any{"state": "present", "docker": map[string]any{"id": "n-bridge", "name": "bridge", "driver": "bridge"}})
			require.NoError(t, err)

			bridge := kind.Observation{Kind: networkKind.Name, Owners: []kind.Reference{{Kind: vmKind.Name, UUID: "vm-1"}}, Status: bridgeStatus}

			// its node has beaten on for minutes, its dockerd listing nothing:
			// the web server's record goes unheard all that time.
			require.NoError(t, workload.Reconcile.Execute(ctx))

			c, err := stored.GetOne(ctx, containerKind.Name, "c-web")
			require.NoError(t, err, "a reset record is kept until the restored disk is looked into, however long its dockerd takes")
			require.True(t, c.Reset)

			stack := graph.Ref{Kind: stackKind.Name, UUID: "s-1"}

			run := func() {
				t.Helper()

				bounded, cancel := context.WithTimeout(ctx, 2*time.Second)
				defer cancel()

				_ = workload.Graphs.Reconcile(bounded, stack)
			}

			// the first look, said by the bridge network every dockerd has.
			beat(first, bridge)

			if late {
				// the loop and the stack's run, its vm heard of long since,
				// before the look's heartbeat of the web server is heard.
				require.NoError(t, workload.Reconcile.Execute(ctx))
				run()

				s, err := stored.GetOne(ctx, stackKind.Name, "s-1")
				require.NoError(t, err, "the stack is not forgotten while a record of its web server is left, reset")
				require.True(t, s.Reset)

				beat(first, web)
			} else {
				// the look's own heartbeat of the web server is lost; the beat
				// after it says it.
				beat(first.Add(15*time.Second), web)
			}

			c, err = stored.GetOne(ctx, containerKind.Name, "c-web")
			require.NoError(t, err)
			assert.False(t, c.Reset, "the web server is found in the restored disk, under its uuid")
			assert.Equal(t, []kind.Reference{{Kind: stackKind.Name, UUID: "s-1"}}, c.Metadata.Owners, "the stack's, as it was")

			run()

			s, err := stored.GetOne(ctx, stackKind.Name, "s-1")
			require.NoError(t, err, "the stack is kept")
			assert.False(t, s.Reset, "held by what the restored disk holds of it")
		})
	}
}

// blinking keeps records as the repository it wraps does, but fails, once
// armed, the one listing of the containers a stack owns: the database gone
// for a moment as a heartbeat is taken.
type blinking struct {
	resource.Repository

	stack string
	armed atomic.Bool
}

func (b *blinking) GetAll(ctx context.Context, kindName string, filter resource.Filter, offset uint, limit uint) ([]resource.Record, uint, error) {
	if kindName == containerKind.Name && filter.Parent.Kind == stackKind.Name && filter.Parent.UUID == b.stack && b.armed.CompareAndSwap(true, false) {
		return nil, 0, errors.New("the database is gone for a moment")
	}

	return b.Repository.GetAll(ctx, kindName, filter, offset, limit)
}

// TestNewControlPlaneWorkload_firstLookHeardAgain holds the control plane to
// failing a heartbeat of the first look into a restored Docker VM that could
// not take in what that look found with no record, the database gone for a
// moment: heard again, as JetStream redelivers what was failed, it takes it
// in, as the stack's whose container it is, which is held by it. The stack's
// web server was made again since the VM's snapshot, under another uuid, so
// the restored disk's own is one no record speaks for.
func TestNewControlPlaneWorkload_firstLookHeardAgain(t *testing.T) {
	t.Parallel()

	ctx := context.Background()

	stored := resourcesMemory.NewRepository()
	records := &blinking{Repository: stored, stack: "s-1"}

	stores := storesOf(stored)
	stores.Resources = records
	stores.Nodes = nodesMemory.NewRepository(vmtest.Alive(vmtest.Node))

	workload, err := NewControlPlaneWorkload(configs.NewWorkloadControlPlane(), stores, nil, &messagingMock.Recorder{}, slog.New(slog.DiscardHandler))
	require.NoError(t, err)

	restoredAt := time.Now().Add(-10 * time.Minute)
	first := restoredAt.Add(3 * time.Minute)
	resetAt := restoredAt.Truncate(time.Millisecond)

	v := vmtest.In(vmtest.Docker("vm-1", "owner"), func(v *vmKind.VM) {
		v.Status.RestoredAt = restoredAt
		v.Status.ObservedAt = time.Now()
	})
	raw, err := kind.Encode(v)
	require.NoError(t, err)

	_, err = stored.Create(ctx, resource.Record{Raw: raw})
	require.NoError(t, err)

	raw, err = kind.Encode(stackKind.Stack{
		Kind:     stackKind.Name,
		Metadata: kind.Metadata{UUID: "s-1", Name: "shop", Slug: "shop", OwnerUUID: "owner", Refs: []kind.Reference{{Kind: vmKind.Name, UUID: "vm-1"}}, CreatedAt: restoredAt.Add(-time.Hour)},
		Spec:     stackKind.Spec{VM: stackKind.VMChoice{UUID: "vm-1"}, Compose: "services:\n  web:\n    image: nginx:1.27\n"},
		Status:   stackKind.Status{Status: kind.Status{State: stackKind.Running, Expected: stackKind.Running, Since: restoredAt.Add(-time.Hour)}},
	})
	require.NoError(t, err)

	_, err = stored.Create(ctx, resource.Record{Raw: raw, Reset: true, ResetAt: resetAt})
	require.NoError(t, err)

	// the web server made again since the snapshot, which the restored disk
	// does not hold.
	_, err = stored.Create(ctx, resource.Record{Raw: blockstest.AContainer("made-since", "vm-1", containerKind.Running, containerKind.Running, func(c *containerKind.Container) {
		c.Metadata.Name = "shop_web"
		c.Metadata.Owners = []kind.Reference{{Kind: stackKind.Name, UUID: "s-1"}}
		c.Metadata.Labels = map[string]string{stackKind.LabelStack: "s-1", stackKind.LabelService: "web"}
		c.Spec.Name = "shop_web"
		c.Status.Docker.Name = "shop_web"
		c.Status.ObservedAt = restoredAt
	}), Reset: true, ResetAt: resetAt})
	require.NoError(t, err)

	// the one the restored disk holds, under the uuid it had then.
	spec, err := containerKind.SpecLabel(containerKind.Spec{Name: "shop_web", Image: "nginx:1.27"})
	require.NoError(t, err)

	labels := blockKinds.Labels(containerKind.Name, "from-the-snapshot")
	labels[containerKind.LabelSpec] = spec
	labels[stackKind.LabelStack] = "s-1"
	labels[stackKind.LabelService] = "web"

	status, err := json.Marshal(containerKind.Status{Status: kind.Status{State: containerKind.Running}, Docker: &containerKind.Docker{ID: "d-web", Name: "shop_web", State: "running", Labels: labels}})
	require.NoError(t, err)

	heartbeat, err := json.Marshal(kind.Heartbeat{Node: vmtest.Node, At: first, Observed: kind.Observation{Kind: containerKind.Name, UUID: "from-the-snapshot", Owners: []kind.Reference{{Kind: vmKind.Name, UUID: "vm-1"}}, Status: status}})
	require.NoError(t, err)

	handler := workload.Subscribers[kind.HeartbeatName(containerKind.Name)]

	// the first look's heartbeat of it, the database gone for a moment as it
	// is taken in.
	records.armed.Store(true)

	err = handler.Handle(ctx, heartbeat)
	require.ErrorIs(t, err, kind.ErrHearAgain, "failed, to be heard again")
	require.False(t, records.armed.Load())

	_, err = stored.GetOne(ctx, containerKind.Name, "from-the-snapshot")
	require.ErrorIs(t, err, domain.ErrNotExists, "nothing taken in yet")

	// heard again.
	require.NoError(t, handler.Handle(ctx, heartbeat))

	adopted, err := stored.GetOne(ctx, containerKind.Name, "from-the-snapshot")
	require.NoError(t, err, "taken in, heard again")
	assert.Equal(t, []kind.Reference{{Kind: stackKind.Name, UUID: "s-1"}}, adopted.Metadata.Owners, "as the stack's")

	bounded, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()

	_ = workload.Graphs.Reconcile(bounded, graph.Ref{Kind: stackKind.Name, UUID: "s-1"})

	s, err := stored.GetOne(ctx, stackKind.Name, "s-1")
	require.NoError(t, err, "the stack is kept")
	assert.False(t, s.Reset, "held by what the restored disk holds of it")
}
