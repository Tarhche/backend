package vm

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/khanzadimahdi/testproject/domain"
	"github.com/khanzadimahdi/testproject/domain/workload/kind"
	snapshotKind "github.com/khanzadimahdi/testproject/domain/workload/kinds/snapshot"
	vmKind "github.com/khanzadimahdi/testproject/domain/workload/kinds/vm"
	"github.com/khanzadimahdi/testproject/domain/workload/noderequest"
	"github.com/khanzadimahdi/testproject/domain/workload/port"
	"github.com/khanzadimahdi/testproject/domain/workload/vm"
	storageMemory "github.com/khanzadimahdi/testproject/infrastructure/storage/memory"
	"github.com/khanzadimahdi/testproject/infrastructure/workload/vm/memory"
)

const (
	gib      = 1 << 30
	nodeName = "workload-orchestrator-01"
)

// gauges keeps what the node published for the dashboards.
type gauges struct {
	lock   sync.Mutex
	up     []bool
	counts []map[vm.State]int
	info   []vm.Info
}

func (g *gauges) Node(_ context.Context, _ string, info vm.Info, counts map[vm.State]int) {
	g.lock.Lock()
	defer g.lock.Unlock()

	g.info = append(g.info, info)
	g.counts = append(g.counts, counts)
}

func (g *gauges) VMHost(_ context.Context, _ string, up bool) {
	g.lock.Lock()
	defer g.lock.Unlock()

	g.up = append(g.up, up)
}

// connections keeps which VMs what was open into them was let go of.
type connections struct {
	lock   sync.Mutex
	forgot []string
}

func (c *connections) Forget(vmUUID string) {
	c.lock.Lock()
	defer c.lock.Unlock()

	c.forgot = append(c.forgot, vmUUID)
}

func (c *connections) forgotten() []string {
	c.lock.Lock()
	defer c.lock.Unlock()

	return append([]string(nil), c.forgot...)
}

// fixture is a node's vm strategy over an engine and a bucket kept in
// memory, on a clock the test moves.
type fixture struct {
	engine      *memory.Engine
	archives    *storageMemory.Storage
	gauges      *gauges
	connections *connections
	node        *Node

	lock  sync.Mutex
	clock time.Time
}

func newFixture(t *testing.T, options ...memory.Option) *fixture {
	t.Helper()

	f := &fixture{
		archives:    storageMemory.New(),
		gauges:      &gauges{},
		connections: &connections{},
		clock:       time.Date(2026, 10, 6, 12, 0, 0, 0, time.UTC),
	}

	f.engine = memory.New(append([]memory.Option{memory.WithCapacity(16, 64*gib, 1000*gib), memory.WithClock(f.now)}, options...)...)
	f.node = New(f.engine, f.archives, f.connections, f.gauges, nodeName)
	f.node.now = f.now

	return f
}

func (f *fixture) now() time.Time {
	f.lock.Lock()
	defer f.lock.Unlock()

	return f.clock
}

func (f *fixture) pass(d time.Duration) {
	f.lock.Lock()
	defer f.lock.Unlock()

	f.clock = f.clock.Add(d)
}

// execute carries action out on v, as the node's command handler does.
func (f *fixture) execute(t *testing.T, v vmKind.VM, action string, payload any) vmKind.Status {
	t.Helper()

	outcome, err := f.node.Execute(context.Background(), v, action, payload)
	require.NoError(t, err)

	return outcome.Status
}

// made is v made on the node, as its create leaves it.
func (f *fixture) made(t *testing.T, v vmKind.VM) {
	t.Helper()

	f.execute(t, v, vmKind.ActionCreate, nil)
}

// aVM is a machine of owner-uuid's, as the control plane records it when it
// sends it a command, in state.
func aVM(uuid string, state kind.State, changes ...func(v *vmKind.VM)) vmKind.VM {
	v := vmKind.VM{
		Kind: vmKind.Name,
		Metadata: kind.Metadata{
			UUID:      uuid,
			Name:      "box",
			Slug:      "box-" + uuid,
			OwnerUUID: "owner-uuid",
			Labels:    map[string]string{vmKind.LabelFlavor: string(vmKind.FlavorMachine)},
			Node:      nodeName,
		},
		Spec: vmKind.Spec{
			Flavor:         vmKind.FlavorMachine,
			Image:          "ubuntu:24.04",
			Resources:      vmKind.Resources{CPUs: 2, Memory: gib, Disk: 10 * gib},
			Ports:          []port.Port{80, 8080},
			Network:        vmKind.Network{Ingress: vm.AccessAllow, Egress: vm.AccessDeny},
			PersistentDisk: true,
		},
		Status: vmKind.Status{Status: kind.Status{State: state, Expected: vmKind.Running}},
	}

	for _, change := range changes {
		change(&v)
	}

	return v
}

// snapshotOf is a snapshot of a VM whose disk held disk, stored in the
// bucket under snapshotUUID.
func (f *fixture) snapshotOf(t *testing.T, snapshotUUID string, disk string) {
	t.Helper()

	source := aVM("source-"+snapshotUUID, vmKind.Scheduled)
	f.made(t, source)
	require.NoError(t, f.engine.SetDisk(source.Metadata.UUID, []byte(disk)))

	var archive bytes.Buffer
	_, err := f.engine.Snapshot(context.Background(), source.Metadata.UUID, &archive)
	require.NoError(t, err)

	require.NoError(t, f.archives.Store(context.Background(), snapshotKind.ObjectKey(snapshotUUID), &archive, int64(archive.Len())))
	require.NoError(t, f.engine.Delete(context.Background(), source.Metadata.UUID))
}

func TestNode_Execute(t *testing.T) {
	t.Parallel()

	ctx := context.Background()

	t.Run("a vm created is made with what it was given, and runs", func(t *testing.T) {
		t.Parallel()

		f := newFixture(t)
		v := aVM("01", vmKind.Scheduled)

		status := f.execute(t, v, vmKind.ActionCreate, nil)

		assert.Equal(t, vmKind.Running, status.State)
		assert.Equal(t, f.now(), status.StartedAt)
		require.NotNil(t, status.Applied, "it was given its config")
		assert.Equal(t, v.Spec.Config(), *status.Applied)
		assert.Equal(t, []vmKind.Endpoint{{Port: 80, Address: "vmhost:20000"}, {Port: 8080, Address: "vmhost:20001"}}, status.Endpoints)

		spec, err := f.engine.Spec("01")
		require.NoError(t, err)
		assert.Equal(t, Spec(v), spec)
		assert.Equal(t, map[string]string{
			vm.LabelOwner:   "owner-uuid",
			vm.LabelVM:      "01",
			vm.LabelSlug:    "box-01",
			vm.LabelPurpose: vm.PurposeVM,
		}, spec.Labels, "what a node tells its VMs by, and opens a terminal for")
		assert.Equal(t, vm.Resources{CPUs: 2, Memory: gib, Disk: 10 * gib}, spec.Resources)
		assert.Equal(t, vm.Network{Ingress: vm.AccessAllow, Egress: vm.AccessDeny}, spec.Network)
		assert.True(t, spec.PersistentDisk)
	})

	t.Run("a docker vm is labelled as one, which is how its node tells its dockerd from the rest", func(t *testing.T) {
		t.Parallel()

		f := newFixture(t)
		f.made(t, aVM("01", vmKind.Scheduled, func(v *vmKind.VM) { v.Spec.Flavor = vmKind.FlavorDocker }))

		spec, err := f.engine.Spec("01")
		require.NoError(t, err)
		assert.Equal(t, vm.KindDocker, spec.Kind)
		assert.Equal(t, "true", spec.Labels[vmKind.LabelDocker])
	})

	t.Run("a vm created from a snapshot is made with the snapshot's disk", func(t *testing.T) {
		t.Parallel()

		f := newFixture(t)
		f.snapshotOf(t, "snapshot-uuid", "what was on it")

		status := f.execute(t, aVM("01", vmKind.Scheduled, func(v *vmKind.VM) { v.Spec.Source = &vmKind.Source{Snapshot: "snapshot-uuid"} }), vmKind.ActionCreate, nil)

		assert.Equal(t, vmKind.Running, status.State)
		assert.NotNil(t, status.Applied)

		disk, err := f.engine.Disk("01")
		require.NoError(t, err)
		assert.Equal(t, "what was on it", string(disk))
	})

	t.Run("one whose snapshot cannot be read is not made, and says why", func(t *testing.T) {
		t.Parallel()

		f := newFixture(t)

		_, err := f.node.Execute(ctx, aVM("01", vmKind.Scheduled, func(v *vmKind.VM) { v.Spec.Source = &vmKind.Source{Snapshot: "gone"} }), vmKind.ActionCreate, nil)
		assert.ErrorContains(t, err, "the snapshot cannot be read")

		_, err = f.engine.Inspect(ctx, "01")
		assert.ErrorIs(t, err, domain.ErrNotExists)
	})

	t.Run("a create delivered again leaves the vm as it is", func(t *testing.T) {
		t.Parallel()

		f := newFixture(t)
		v := aVM("01", vmKind.Scheduled)
		f.made(t, v)
		require.NoError(t, f.engine.SetDisk("01", []byte("written since")))

		status := f.execute(t, v, vmKind.ActionCreate, nil)
		assert.Equal(t, vmKind.Running, status.State)

		disk, err := f.engine.Disk("01")
		require.NoError(t, err)
		assert.Equal(t, "written since", string(disk), "it was not made again")
	})

	t.Run("a stopped vm is started, and one its node lost is made again", func(t *testing.T) {
		t.Parallel()

		f := newFixture(t)
		f.made(t, aVM("01", vmKind.Scheduled))
		require.NoError(t, f.engine.Stop(ctx, "01"))

		assert.Equal(t, vmKind.Running, f.execute(t, aVM("01", vmKind.Starting), vmKind.ActionStart, nil).State)

		lost := f.execute(t, aVM("02", vmKind.Starting), vmKind.ActionStart, nil)
		assert.Equal(t, vmKind.Running, lost.State)
		assert.NotNil(t, lost.Applied, "it was made, with its config")
	})

	t.Run("a running vm is stopped, and what was open into it let go of", func(t *testing.T) {
		t.Parallel()

		f := newFixture(t)
		f.made(t, aVM("01", vmKind.Scheduled))

		status := f.execute(t, aVM("01", vmKind.Stopping), vmKind.ActionStop, nil)

		assert.Equal(t, vmKind.Stopped, status.State)
		assert.Nil(t, status.Applied, "it was given nothing")
		assert.Contains(t, f.connections.forgotten(), "01")

		instance, err := f.engine.Inspect(ctx, "01")
		require.NoError(t, err)
		assert.Equal(t, vm.InstanceStopped, instance.State)

		assert.Equal(t, vmKind.Stopped, f.execute(t, aVM("01", vmKind.Stopping), vmKind.ActionStop, nil).State, "a stopped one is what was asked for already")
	})

	t.Run("one its node holds nothing of is not running, and one that failed says nothing", func(t *testing.T) {
		t.Parallel()

		f := newFixture(t)

		assert.Equal(t, vmKind.Status{Status: kind.Status{State: vmKind.Stopped}}, f.execute(t, aVM("01", vmKind.Stopping), vmKind.ActionStop, nil))
		assert.Equal(t, vmKind.Status{}, f.execute(t, aVM("01", vmKind.Failed), vmKind.ActionStop, nil), "it keeps saying why it failed")
	})

	t.Run("a running vm is restarted in place, and a stopped one started", func(t *testing.T) {
		t.Parallel()

		f := newFixture(t)
		f.made(t, aVM("01", vmKind.Scheduled))

		f.pass(time.Minute)

		restarted := f.execute(t, aVM("01", vmKind.Restarting), vmKind.ActionRestart, nil)
		assert.Equal(t, vmKind.Running, restarted.State)
		assert.Equal(t, f.now(), restarted.StartedAt, "it started again")
		assert.Contains(t, f.connections.forgotten(), "01")

		require.NoError(t, f.engine.Stop(ctx, "01"))
		assert.Equal(t, vmKind.Running, f.execute(t, aVM("01", vmKind.Starting), vmKind.ActionRestart, nil).State)

		assert.Equal(t, vmKind.Running, f.execute(t, aVM("02", vmKind.Starting), vmKind.ActionRestart, nil).State, "and one its node holds nothing of is made")
	})

	t.Run("a vm reconfigured is given its ports, network and resources, and says it was", func(t *testing.T) {
		t.Parallel()

		f := newFixture(t)
		f.made(t, aVM("01", vmKind.Scheduled))

		changed := aVM("01", vmKind.Restarting, func(v *vmKind.VM) {
			v.Spec.Ports = []port.Port{443}
			v.Spec.Network.Egress = vm.AccessAllow
			v.Spec.Resources.Memory = 2 * gib
		})

		status := f.execute(t, changed, vmKind.ActionReconfigure, nil)
		assert.Equal(t, vmKind.Running, status.State)
		require.NotNil(t, status.Applied)
		assert.Equal(t, changed.Spec.Config(), *status.Applied)

		spec, err := f.engine.Spec("01")
		require.NoError(t, err)
		assert.Equal(t, []port.Port{443}, spec.Ports)
		assert.Equal(t, vm.AccessAllow, spec.Network.Egress)
		assert.Equal(t, uint64(2*gib), spec.Resources.Memory)
	})

	t.Run("one its node holds nothing of is as good as reconfigured: it is made with its config", func(t *testing.T) {
		t.Parallel()

		f := newFixture(t)
		v := aVM("01", vmKind.Stopped)

		status := f.execute(t, v, vmKind.ActionReconfigure, nil)
		assert.Equal(t, vmKind.Stopped, status.State)
		require.NotNil(t, status.Applied)
		assert.Equal(t, v.Spec.Config(), *status.Applied)
	})

	t.Run("a vm restored has the snapshot's disk, keeps what it was given, and runs", func(t *testing.T) {
		t.Parallel()

		f := newFixture(t)
		f.snapshotOf(t, "snapshot-uuid", "what was on it")

		f.made(t, aVM("01", vmKind.Scheduled))
		require.NoError(t, f.engine.SetDisk("01", []byte("written since")))

		status := f.execute(t, aVM("01", vmKind.Restoring), vmKind.ActionRestore, vmKind.RestorePayload{SnapshotUUID: "snapshot-uuid"})
		assert.Equal(t, vmKind.Running, status.State)
		assert.NotNil(t, status.Applied)
		assert.Contains(t, f.connections.forgotten(), "01")

		disk, err := f.engine.Disk("01")
		require.NoError(t, err)
		assert.Equal(t, "what was on it", string(disk))

		spec, err := f.engine.Spec("01")
		require.NoError(t, err)
		assert.Equal(t, "box-01", spec.Labels[vm.LabelSlug], "it keeps its slug")
		assert.Equal(t, []port.Port{80, 8080}, spec.Ports, "and its ports")
	})

	t.Run("a restore from a snapshot that cannot be read fails", func(t *testing.T) {
		t.Parallel()

		f := newFixture(t)
		f.made(t, aVM("01", vmKind.Scheduled))

		_, err := f.node.Execute(ctx, aVM("01", vmKind.Restoring), vmKind.ActionRestore, vmKind.RestorePayload{SnapshotUUID: "gone"})
		assert.ErrorContains(t, err, "the snapshot cannot be read")
	})

	t.Run("a vm deleted is gone, disk and all, and one that was not here is gone already", func(t *testing.T) {
		t.Parallel()

		f := newFixture(t)
		f.made(t, aVM("01", vmKind.Scheduled))

		f.execute(t, aVM("01", vmKind.Deleting), vmKind.ActionDelete, nil)

		_, err := f.engine.Inspect(ctx, "01")
		assert.ErrorIs(t, err, domain.ErrNotExists)
		assert.Contains(t, f.connections.forgotten(), "01")

		f.execute(t, aVM("02", vmKind.Deleting), vmKind.ActionDelete, nil)
	})

	t.Run("nothing else is done to a vm on its node", func(t *testing.T) {
		t.Parallel()

		_, err := newFixture(t).node.Execute(ctx, aVM("01", vmKind.Running), vmKind.ActionUpdate, nil)
		assert.ErrorIs(t, err, kind.ErrUnknownAction)
	})

	t.Run("an engine that fails a command fails it", func(t *testing.T) {
		t.Parallel()

		f := newFixture(t)
		big := aVM("01", vmKind.Scheduled, func(v *vmKind.VM) { v.Spec.Resources.Memory = 65 * gib })

		_, err := f.node.Execute(ctx, big, vmKind.ActionCreate, nil)
		assert.ErrorIs(t, err, vm.ErrNoCapacity)
	})
}

func TestNode_Query(t *testing.T) {
	t.Parallel()

	ctx := context.Background()

	t.Run("a vm's log is read from its engine, from a moment, its last lines", func(t *testing.T) {
		t.Parallel()

		f := newFixture(t)
		v := aVM("01", vmKind.Running)
		f.made(t, v)

		for i := range 5 {
			f.pass(time.Second)
			require.NoError(t, f.engine.Log("01", vm.LogSourceKernel, fmt.Sprintf("line %d", i)))
		}

		answer, err := f.node.Query(ctx, v, vmKind.ActionLogs, vmKind.LogsPayload{Since: f.now().Add(-3 * time.Second), Tail: 2})
		require.NoError(t, err)

		logs := answer.(vmKind.Logs)
		require.Len(t, logs.Lines, 2)
		assert.Equal(t, "line 3", logs.Lines[0].Line)
		assert.Equal(t, "line 4", logs.Lines[1].Line)
		assert.Equal(t, vm.LogSourceKernel, logs.Lines[1].Source)
		assert.False(t, logs.Truncated)
	})

	t.Run("one longer than an answer carries is cut to its end, and says so", func(t *testing.T) {
		t.Parallel()

		f := newFixture(t)
		v := aVM("01", vmKind.Running)
		f.made(t, v)

		for i := range noderequest.MaxLogLines + 3 {
			require.NoError(t, f.engine.Log("01", vm.LogSourceMain, fmt.Sprintf("line %d", i)))
		}

		answer, err := f.node.Query(ctx, v, vmKind.ActionLogs, vmKind.LogsPayload{})
		require.NoError(t, err)

		logs := answer.(vmKind.Logs)
		require.NotEmpty(t, logs.Lines)
		assert.LessOrEqual(t, len(logs.Lines), noderequest.MaxLogLines)
		assert.Equal(t, fmt.Sprintf("line %d", noderequest.MaxLogLines+2), logs.Lines[len(logs.Lines)-1].Line, "the last line is kept")
		assert.True(t, logs.Truncated)
	})

	t.Run("one whose lines do not fit in one answer is cut to what fits", func(t *testing.T) {
		t.Parallel()

		f := newFixture(t)
		v := aVM("01", vmKind.Running)
		f.made(t, v)

		long := strings.Repeat("x", 64<<10)
		for range 40 {
			require.NoError(t, f.engine.Log("01", vm.LogSourceMain, long))
		}

		answer, err := f.node.Query(ctx, v, vmKind.ActionLogs, vmKind.LogsPayload{Tail: 40})
		require.NoError(t, err)

		logs := answer.(vmKind.Logs)
		assert.Less(t, len(logs.Lines), 40)
		assert.True(t, logs.Truncated)
	})

	t.Run("a sample of what a running vm uses, its cpu held to 0 to 100", func(t *testing.T) {
		t.Parallel()

		f := newFixture(t)
		v := aVM("01", vmKind.Running)
		f.made(t, v)
		require.NoError(t, f.engine.SetStats("01", vm.Stats{CPUPercent: 130, MemoryUsed: 256 << 20}))

		answer, err := f.node.Query(ctx, v, vmKind.ActionStats, nil)
		require.NoError(t, err)

		stats := answer.(*vmKind.Stats)
		assert.Equal(t, 100.0, stats.CPUPercent)
		assert.Equal(t, uint64(256<<20), stats.MemoryUsed)
		assert.Equal(t, uint64(gib), stats.MemoryLimit)
	})

	t.Run("a vm its node holds nothing of is not running", func(t *testing.T) {
		t.Parallel()

		f := newFixture(t)
		v := aVM("01", vmKind.Running)

		_, err := f.node.Query(ctx, v, vmKind.ActionLogs, vmKind.LogsPayload{})
		assert.ErrorIs(t, err, vm.ErrNotRunning)

		_, err = f.node.Query(ctx, v, vmKind.ActionStats, nil)
		assert.ErrorIs(t, err, vm.ErrNotRunning)
	})

	t.Run("nothing else is asked of a vm on its node", func(t *testing.T) {
		t.Parallel()

		_, err := newFixture(t).node.Query(ctx, aVM("01", vmKind.Running), vmKind.ActionState, nil)
		assert.ErrorIs(t, err, kind.ErrUnknownAction)
	})
}

// failingStats is an engine that cannot sample anything.
type failingStats struct {
	vm.Engine
}

func (failingStats) Stats(context.Context, string) (vm.Stats, error) {
	return vm.Stats{}, errors.New("the engine is busy")
}

// failingInfo is an engine that cannot say what it offers.
type failingInfo struct {
	vm.Engine
}

func (failingInfo) Info(context.Context) (vm.Info, error) {
	return vm.Info{}, errors.New("the vmhost is down")
}

func TestNode_State(t *testing.T) {
	t.Parallel()

	ctx := context.Background()

	find := func(t *testing.T, report kind.Report[vmKind.Status], uuid string) vmKind.Status {
		t.Helper()

		observed, found := report.Find(uuid)
		require.True(t, found, "%s is reported", uuid)

		return observed.Status
	}

	t.Run("every vm its engine holds is reported, as its engine says it is, and nothing else", func(t *testing.T) {
		t.Parallel()

		f := newFixture(t)
		f.made(t, aVM("03", vmKind.Scheduled))
		f.made(t, aVM("01", vmKind.Scheduled))
		f.made(t, aVM("02", vmKind.Scheduled))
		require.NoError(t, f.engine.Stop(ctx, "02"))
		require.NoError(t, f.engine.Fail("03", ""))

		// a task's VM is the engine's too, and not a VM of the kind's.
		_, err := f.engine.Create(ctx, vm.Spec{ID: "task", Kind: vm.KindMachine, Resources: vm.Resources{CPUs: 1, Memory: gib, Disk: gib}, Labels: map[string]string{vm.LabelPurpose: "task"}})
		require.NoError(t, err)

		report, err := f.node.State(ctx)
		require.NoError(t, err)

		uuids := make([]string, len(report.Instances))
		for i, instance := range report.Instances {
			uuids[i] = instance.UUID
		}

		assert.Equal(t, []string{"01", "02", "03"}, uuids, "in order, and only VMs")

		running := find(t, report, "01")
		assert.Equal(t, vmKind.Running, running.State)
		assert.Equal(t, f.now(), running.StartedAt)
		assert.Len(t, running.Endpoints, 2)
		require.NotNil(t, running.Stats, "a running one is sampled")

		stopped := find(t, report, "02")
		assert.Equal(t, vmKind.Stopped, stopped.State)
		assert.Nil(t, stopped.Stats)

		failed := find(t, report, "03")
		assert.Equal(t, vmKind.Failed, failed.State)
		assert.Equal(t, "the vm failed", failed.Reason, "one whose engine did not say why")

		assert.Equal(t, []bool{true}, f.gauges.up, "its engine answered")
		require.Len(t, f.gauges.counts, 1)
		assert.Equal(t, map[vm.State]int{vm.Running: 1, vm.Stopped: 1, vm.Failed: 1}, f.gauges.counts[0], "its VMs, and not what else its engine holds")
		assert.Equal(t, uint(16), f.gauges.info[0].CPUs)
	})

	t.Run("a running vm is sampled every other beat, and its sample shown in between", func(t *testing.T) {
		t.Parallel()

		f := newFixture(t)
		f.made(t, aVM("01", vmKind.Scheduled))
		require.NoError(t, f.engine.SetStats("01", vm.Stats{CPUPercent: 10}))

		report, err := f.node.State(ctx)
		require.NoError(t, err)
		assert.Equal(t, 10.0, find(t, report, "01").Stats.CPUPercent)

		require.NoError(t, f.engine.SetStats("01", vm.Stats{CPUPercent: 20}))

		f.pass(time.Second)

		report, err = f.node.State(ctx)
		require.NoError(t, err)
		assert.Equal(t, 10.0, find(t, report, "01").Stats.CPUPercent, "not sampled again yet")

		f.pass(time.Second)

		report, err = f.node.State(ctx)
		require.NoError(t, err)
		assert.Equal(t, 20.0, find(t, report, "01").Stats.CPUPercent)
	})

	t.Run("a sample that cannot be taken shows the last one a while, and then none", func(t *testing.T) {
		t.Parallel()

		f := newFixture(t)
		f.made(t, aVM("01", vmKind.Scheduled))
		require.NoError(t, f.engine.SetStats("01", vm.Stats{CPUPercent: 10}))

		_, err := f.node.State(ctx)
		require.NoError(t, err)

		f.node.engine = failingStats{Engine: f.engine}

		f.pass(5 * time.Second)

		report, err := f.node.State(ctx)
		require.NoError(t, err)
		require.NotNil(t, find(t, report, "01").Stats)
		assert.Equal(t, 10.0, find(t, report, "01").Stats.CPUPercent)

		f.pass(10 * time.Second)

		report, err = f.node.State(ctx)
		require.NoError(t, err)
		assert.Nil(t, find(t, report, "01").Stats)
	})

	t.Run("a vm a command is carried out on is in flight, as the command left it", func(t *testing.T) {
		t.Parallel()

		f := newFixture(t)
		restarting := aVM("01", vmKind.Restarting)
		f.made(t, restarting)

		f.node.begin(restarting)

		report, err := f.node.State(ctx)
		require.NoError(t, err)
		assert.Equal(t, vmKind.Restarting, find(t, report, "01").State, "what it does halfway through is not what the command came to")

		f.node.end("01")

		report, err = f.node.State(ctx)
		require.NoError(t, err)
		assert.Equal(t, vmKind.Running, find(t, report, "01").State)

		f.node.begin(aVM("01", vmKind.Stopped))

		report, err = f.node.State(ctx)
		require.NoError(t, err)
		assert.Equal(t, vmKind.Running, find(t, report, "01").State, "a command that leaves it at rest leaves it what it is doing")
	})

	t.Run("an engine that does not answer is said, and reports nothing", func(t *testing.T) {
		t.Parallel()

		f := newFixture(t)
		f.node.engine = failingInfo{Engine: f.engine}

		_, err := f.node.State(ctx)
		assert.Error(t, err)
		assert.Equal(t, []bool{false}, f.gauges.up)
	})
}

func TestNode_Endpoint(t *testing.T) {
	t.Parallel()

	ctx := context.Background()

	f := newFixture(t)
	f.made(t, aVM("01", vmKind.Scheduled))
	f.made(t, aVM("02", vmKind.Scheduled))
	require.NoError(t, f.engine.Stop(ctx, "02"))
	f.made(t, aVM("03", vmKind.Scheduled, func(v *vmKind.VM) { v.Spec.Network.Ingress = vm.AccessDeny }))

	for name, tt := range map[string]struct {
		slug string
		port port.Port
		want kind.Endpoint
		err  error
	}{
		"a port a vm exposes":                     {slug: "box-01", port: 8080, want: kind.Endpoint{Port: 8080, Address: "vmhost:20001"}},
		"its lowest, when none is named":          {slug: "box-01", want: kind.Endpoint{Port: 80, Address: "vmhost:20000"}},
		"one it does not expose is not there":     {slug: "box-01", port: 22, err: domain.ErrNotExists},
		"nor is a vm this node does not hold":     {slug: "box-09", port: 80, err: domain.ErrNotExists},
		"one that is not running cannot be asked": {slug: "box-02", port: 80, err: kind.ErrUnreachable},
		"one that lets nothing in exposes none":   {slug: "box-03", port: 80, err: domain.ErrNotExists},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			endpoint, err := f.node.Endpoint(ctx, tt.slug, tt.port)
			if tt.err != nil {
				assert.ErrorIs(t, err, tt.err)

				return
			}

			require.NoError(t, err)
			assert.Equal(t, tt.want, endpoint)
		})
	}
}

func TestNode_Attach(t *testing.T) {
	t.Parallel()

	ctx := context.Background()

	opened := make(chan vm.ExecOptions, 1)

	f := newFixture(t, memory.WithExec(func(_ context.Context, _ string, options vm.ExecOptions, _ io.Reader, stdout io.Writer, _ io.Writer) int {
		opened <- options
		_, _ = io.WriteString(stdout, "$ ")

		return 0
	}))
	f.made(t, aVM("01", vmKind.Scheduled))
	f.made(t, aVM("02", vmKind.Scheduled))
	require.NoError(t, f.engine.Stop(ctx, "02"))

	t.Run("its owner opens a shell in it, which outlives the request that opened it", func(t *testing.T) {
		t.Parallel()

		asking, done := context.WithCancel(ctx)

		session, err := f.node.Attach(asking, vmKind.ActionAttach, "01", "owner-uuid")
		require.NoError(t, err)

		done()

		said, err := io.ReadAll(session.Stdout())
		require.NoError(t, err)
		assert.Equal(t, "$ ", string(said))

		code, err := session.Wait(ctx)
		require.NoError(t, err)
		assert.Zero(t, code)

		options := <-opened
		assert.Equal(t, shell, options.Command)
		assert.True(t, options.TTY)
	})

	for name, tt := range map[string]struct {
		action string
		uuid   string
		owner  string
		err    error
	}{
		"nobody else does":                 {action: vmKind.ActionAttach, uuid: "01", owner: "other", err: domain.ErrNotExists},
		"nor anybody who says nothing":     {action: vmKind.ActionAttach, uuid: "01", err: domain.ErrNotExists},
		"nobody opens one this node lacks": {action: vmKind.ActionAttach, uuid: "09", owner: "owner-uuid", err: domain.ErrNotExists},
		"nor one in a vm that is stopped":  {action: vmKind.ActionAttach, uuid: "02", owner: "owner-uuid", err: kind.ErrUnreachable},
		"a vm has no other stream":         {action: "tail", uuid: "01", owner: "owner-uuid", err: kind.ErrUnknownAction},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			_, err := f.node.Attach(ctx, tt.action, tt.uuid, tt.owner)
			assert.ErrorIs(t, err, tt.err)
		})
	}
}

func TestUUIDOf(t *testing.T) {
	t.Parallel()

	assert.Equal(t, "vm-uuid", UUIDOf(vm.Instance{ID: "instance", Labels: map[string]string{vm.LabelVM: "vm-uuid"}}))
	assert.Equal(t, "instance", UUIDOf(vm.Instance{ID: "instance"}), "a vm is made under its own uuid")
}
