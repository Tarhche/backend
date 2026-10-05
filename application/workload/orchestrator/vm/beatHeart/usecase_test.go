package beatHeart

import (
	"context"
	"errors"
	"log/slog"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/khanzadimahdi/testproject/domain/workload/vm"
	"github.com/khanzadimahdi/testproject/domain/workload/vm/events"
	messaging "github.com/khanzadimahdi/testproject/infrastructure/messaging/mock"
	"github.com/khanzadimahdi/testproject/infrastructure/repository/mocks/workload/engine"
	memory "github.com/khanzadimahdi/testproject/infrastructure/workload/vm/memory"
)

const nodeName = "workload-orchestrator-01"

// gauged keeps what was published to the dashboards.
type gauged struct {
	lock   sync.Mutex
	info   vm.Info
	counts map[vm.State]int
	up     []bool
}

func (g *gauged) Node(_ context.Context, node string, info vm.Info, counts map[vm.State]int) {
	g.lock.Lock()
	defer g.lock.Unlock()

	g.info, g.counts = info, counts
}

func (g *gauged) VMHost(_ context.Context, node string, up bool) {
	g.lock.Lock()
	defer g.lock.Unlock()

	g.up = append(g.up, up)
}

func spec(id string, purpose string) vm.Spec {
	return vm.Spec{
		ID:        id,
		Kind:      vm.KindMachine,
		Image:     "ubuntu:24.04",
		Resources: vm.Resources{CPUs: 1, Memory: 1 << 30, Disk: 10 << 30},
		Labels:    map[string]string{vm.LabelPurpose: purpose, vm.LabelVM: id},
	}
}

func TestUseCase_Execute(t *testing.T) {
	t.Parallel()

	t.Run("every VM this node holds is reported as its engine sees it, with what the node offers", func(t *testing.T) {
		t.Parallel()

		started := time.Date(2026, 10, 4, 12, 0, 0, 0, time.UTC)
		e := memory.New(memory.WithCapacity(8, 16<<30, 200<<30), memory.WithClock(func() time.Time { return started }))

		for _, s := range []vm.Spec{spec("vm-1", vm.PurposeVM), spec("vm-2", vm.PurposeVM), spec("vm-3", vm.PurposeVM), spec("task-1", vm.PurposeTask)} {
			_, err := e.Create(t.Context(), s)
			require.NoError(t, err)
		}

		require.NoError(t, e.SetStats("vm-1", vm.Stats{CPUPercent: 25, MemoryUsed: 256 << 20}))
		require.NoError(t, e.Stop(t.Context(), "vm-2"))
		require.NoError(t, e.Fail("vm-3", "the kernel panicked"))

		producer := &messaging.Recorder{}
		gauges := &gauged{}

		require.NoError(t, NewUseCase(e, producer, gauges, nodeName, slog.New(slog.DiscardHandler)).Execute(t.Context()))

		beats, err := messaging.Produced[events.VMHeartbeat](producer, events.VMHeartbeatName)
		require.NoError(t, err)
		require.Len(t, beats, 1)

		beat := beats[0]
		assert.Equal(t, nodeName, beat.NodeName)
		assert.Equal(t, events.Info{
			Engine:    "memory",
			Version:   "1",
			CPUs:      8,
			Memory:    16 << 30,
			Disk:      200 << 30,
			Allocated: events.Resources{CPUs: 4, Memory: 4 << 30, Disk: 40 << 30},
		}, beat.Capacity, "what the node has given counts the tasks' instances too")

		assert.Equal(t, []events.VMBeat{
			{
				UUID:      "vm-1",
				State:     vm.InstanceRunning,
				StartedAt: started,
				Stats: events.Stats{
					CPUPercent:  25,
					MemoryUsed:  256 << 20,
					MemoryLimit: 1 << 30,
					DiskTotal:   10 << 30,
					SampledAt:   started,
				},
			},
			{UUID: "vm-2", State: vm.InstanceStopped, StartedAt: started},
			{UUID: "vm-3", State: vm.InstanceFailed, Reason: "the kernel panicked", StartedAt: started},
		}, beat.VMs, "a task's instance is not a VM")

		assert.Equal(t, []bool{true}, gauges.up)
		assert.Equal(t, map[vm.State]int{vm.Running: 1, vm.Stopped: 1, vm.Failed: 1}, gauges.counts)
		assert.Equal(t, uint(8), gauges.info.CPUs)
	})

	t.Run("a node holding no VM says so rather than nothing", func(t *testing.T) {
		t.Parallel()

		producer := &messaging.Recorder{}

		require.NoError(t, NewUseCase(memory.New(), producer, &gauged{}, nodeName, slog.New(slog.DiscardHandler)).Execute(t.Context()))

		require.Len(t, producer.Messages(), 1)
		assert.Contains(t, string(producer.Messages()[0].Payload), `"vms":[]`)
	})

	t.Run("an engine that does not answer is a vmhost that is down, and nothing is said", func(t *testing.T) {
		t.Parallel()

		var e engine.MockEngine
		e.On("Info", mock.Anything).Return(vm.Info{}, errors.New("connection refused"))
		defer e.AssertExpectations(t)

		producer := &messaging.Recorder{}
		gauges := &gauged{}

		err := NewUseCase(&e, producer, gauges, nodeName, slog.New(slog.DiscardHandler)).Execute(t.Context())
		assert.Error(t, err)

		assert.Equal(t, []bool{false}, gauges.up)
		assert.Empty(t, producer.Messages())
	})

	t.Run("a VM that cannot be sampled is still reported", func(t *testing.T) {
		t.Parallel()

		var e engine.MockEngine
		e.On("Info", mock.Anything).Return(vm.Info{CPUs: 2}, nil)
		e.On("List", mock.Anything).Return([]vm.Instance{
			{ID: "vm-1", State: vm.InstanceRunning, Labels: map[string]string{vm.LabelPurpose: vm.PurposeVM, vm.LabelVM: "vm-1"}},
		}, nil)
		e.On("Stats", mock.Anything, "vm-1").Return(vm.Stats{}, errors.New("not now"))
		defer e.AssertExpectations(t)

		producer := &messaging.Recorder{}

		require.NoError(t, NewUseCase(&e, producer, &gauged{}, nodeName, slog.New(slog.DiscardHandler)).Execute(t.Context()))

		beats, err := messaging.Produced[events.VMHeartbeat](producer, events.VMHeartbeatName)
		require.NoError(t, err)
		require.Len(t, beats[0].VMs, 1)
		assert.Equal(t, vm.InstanceRunning, beats[0].VMs[0].State)
	})
}
