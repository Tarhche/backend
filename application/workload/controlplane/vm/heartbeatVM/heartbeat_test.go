package heartbeatVM

import (
	"context"
	"encoding/json"
	"log/slog"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/khanzadimahdi/testproject/application/workload/controlplane/vm/vmtest"
	"github.com/khanzadimahdi/testproject/domain/workload/node"
	"github.com/khanzadimahdi/testproject/domain/workload/vm"
	"github.com/khanzadimahdi/testproject/domain/workload/vm/events"
)

var capacity = events.Info{
	Engine:    "microsandbox",
	Version:   "0.7.6",
	CPUs:      8,
	Memory:    16 * vmtest.GiB,
	Disk:      200 * vmtest.GiB,
	Allocated: events.Resources{CPUs: 1, Memory: vmtest.GiB, Disk: 10 * vmtest.GiB},
}

func handlerOf(w *vmtest.Workload) *Heartbeat {
	logger := slog.New(slog.DiscardHandler)

	return NewHeartbeat(w.VMs, w.Nodes, w.Lifecycle, w.Commander, logger)
}

func heartbeat(t *testing.T, at time.Time, beats ...events.VMBeat) []byte {
	t.Helper()

	payload, err := json.Marshal(events.VMHeartbeat{NodeName: vmtest.Node, Capacity: capacity, VMs: beats, At: at})
	require.NoError(t, err)

	return payload
}

func TestHeartbeat_Handle(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	now := time.Now()

	in := func(state vm.State, change ...func(*vm.VM)) vm.VM {
		v := vmtest.Running("01", "owner")
		v.CurrentState = state
		v.LastHeartbeatAt = now.Add(-5 * time.Second)
		v.UpdatedAt = now.Add(-10 * time.Second)

		for _, c := range change {
			c(&v)
		}

		return v
	}

	for name, tt := range map[string]struct {
		vm        vm.VM
		beat      events.VMBeat
		state     vm.State
		expected  vm.State
		reason    string
		restoreTo string
	}{
		"a vm being made is running once its node says so": {
			vm:       in(vm.Scheduled, func(v *vm.VM) { v.RestoreFrom = "snapshot-uuid" }),
			beat:     events.VMBeat{UUID: "01", State: vm.InstanceRunning, StartedAt: now},
			state:    vm.Running,
			expected: vm.Running,
		},
		"a vm being stopped is not taken back to running by a report from before": {
			vm:        in(vm.Stopping, func(v *vm.VM) { v.ExpectedState = vm.Stopped }),
			beat:      events.VMBeat{UUID: "01", State: vm.InstanceRunning},
			state:     vm.Stopping,
			expected:  vm.Stopped,
			restoreTo: "",
		},
		"and is stopped once its node says so": {
			vm:       in(vm.Stopping, func(v *vm.VM) { v.ExpectedState = vm.Stopped }),
			beat:     events.VMBeat{UUID: "01", State: vm.InstanceStopped},
			state:    vm.Stopped,
			expected: vm.Stopped,
		},
		"a running vm stopped behind the workload's back is stopped": {
			vm:       in(vm.Running),
			beat:     events.VMBeat{UUID: "01", State: vm.InstanceExited},
			state:    vm.Stopped,
			expected: vm.Running,
		},
		"a running vm that fell over is failed, and still wanted running": {
			vm:       in(vm.Running),
			beat:     events.VMBeat{UUID: "01", State: vm.InstanceFailed, Reason: "kernel panic"},
			state:    vm.Failed,
			expected: vm.Running,
			reason:   "kernel panic",
		},
		"a restarting vm that has not started again since is still restarting": {
			vm:       in(vm.Restarting),
			beat:     events.VMBeat{UUID: "01", State: vm.InstanceRunning, StartedAt: now.Add(-time.Hour)},
			state:    vm.Restarting,
			expected: vm.Running,
		},
		"one that has is running": {
			vm:       in(vm.Restarting),
			beat:     events.VMBeat{UUID: "01", State: vm.InstanceRunning, StartedAt: now},
			state:    vm.Running,
			expected: vm.Running,
		},
		"a vm given up on that came back is wanted running again": {
			vm:       in(vm.Failed, func(v *vm.VM) { v.ExpectedState = vm.Failed; v.Reason = "it failed" }),
			beat:     events.VMBeat{UUID: "01", State: vm.InstanceRunning},
			state:    vm.Running,
			expected: vm.Running,
		},
		"a vm being restored waits for the restore to be reported": {
			vm:        in(vm.Restoring, func(v *vm.VM) { v.RestoreFrom = "snapshot-uuid" }),
			beat:      events.VMBeat{UUID: "01", State: vm.InstanceStopped},
			state:     vm.Restoring,
			expected:  vm.Running,
			restoreTo: "snapshot-uuid",
		},
		"an instance that is only created says nothing either way": {
			vm:       in(vm.Starting),
			beat:     events.VMBeat{UUID: "01", State: vm.InstanceCreated},
			state:    vm.Starting,
			expected: vm.Running,
		},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			w := vmtest.New(vmtest.WithVMs(tt.vm))
			tt.beat.Stats = events.Stats{CPUPercent: 12.5, MemoryUsed: 512 * vmtest.MiB}

			require.NoError(t, handlerOf(w).Handle(ctx, heartbeat(t, now, tt.beat)))

			stored, _ := w.VMs.Stored("01")
			assert.Equal(t, tt.state.String(), stored.CurrentState.String(), "what it is")
			assert.Equal(t, tt.expected.String(), stored.ExpectedState.String(), "what is wanted of it")
			assert.Equal(t, tt.reason, stored.Reason)
			assert.Equal(t, tt.restoreTo, stored.RestoreFrom)

			// whatever it is doing, it was spoken for, and what it uses is written down.
			assert.True(t, now.Equal(stored.LastHeartbeatAt))
			assert.Equal(t, 12.5, stored.Stats.CPUPercent)
			assert.Equal(t, tt.vm.UpdatedAt, stored.UpdatedAt, "a report asks for nothing")
		})
	}

	t.Run("the node's capacity is written down, and that it is alive", func(t *testing.T) {
		t.Parallel()

		quiet := vmtest.Alive(vmtest.Node)
		quiet.LastHeartbeatAt = now.Add(-time.Minute)
		quiet.Capacity = vm.Info{}

		w := vmtest.New(vmtest.WithNodes(quiet))

		require.NoError(t, handlerOf(w).Handle(ctx, heartbeat(t, now)))

		stored, err := w.Nodes.GetOne(ctx, vmtest.Node)
		require.NoError(t, err)
		assert.Equal(t, capacity.ToVM(), stored.Capacity)
		assert.True(t, now.Equal(stored.LastHeartbeatAt))
	})

	t.Run("a node nobody has heard of is written down too", func(t *testing.T) {
		t.Parallel()

		w := vmtest.New(vmtest.WithNodes())

		require.NoError(t, handlerOf(w).Handle(ctx, heartbeat(t, now)))

		stored, err := w.Nodes.GetOne(ctx, vmtest.Node)
		require.NoError(t, err)
		assert.Equal(t, node.OrchestratorRole, stored.Role)
		assert.Equal(t, uint(8), stored.Capacity.CPUs)
	})

	t.Run("a vm nobody has a record of is removed from its node", func(t *testing.T) {
		t.Parallel()

		w := vmtest.New()

		require.NoError(t, handlerOf(w).Handle(ctx, heartbeat(t, now, events.VMBeat{UUID: "orphan", State: vm.InstanceRunning})))

		var asked events.VMDeleteRequested
		require.True(t, w.Producer.Last(events.VMDeleteRequestedName, &asked))
		assert.Equal(t, events.VMDeleteRequested{VMUUID: "orphan", NodeName: vmtest.Node}, asked)
	})

	t.Run("a vm on its way out that its node no longer lists is forgotten", func(t *testing.T) {
		t.Parallel()

		w := vmtest.New(vmtest.WithVMs(
			in(vm.Deleting, func(v *vm.VM) { v.ExpectedState = vm.Deleting }),
			func() vm.VM { v := vmtest.Running("02", "owner"); return v }(),
		))

		require.NoError(t, handlerOf(w).Handle(ctx, heartbeat(t, now, events.VMBeat{UUID: "02", State: vm.InstanceRunning})))

		_, kept := w.VMs.Stored("01")
		assert.False(t, kept, "it is gone")

		_, kept = w.VMs.Stored("02")
		assert.True(t, kept, "one that is not on its way out is not forgotten for being listed")
	})

	t.Run("what will never be handled is not handed back", func(t *testing.T) {
		t.Parallel()

		w := vmtest.New()

		assert.NoError(t, handlerOf(w).Handle(ctx, []byte("{")))
	})
}
