package lifecycle_test

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/khanzadimahdi/testproject/application/workload/controlplane/vm/lifecycle"
	"github.com/khanzadimahdi/testproject/application/workload/controlplane/vm/vmtest"
	"github.com/khanzadimahdi/testproject/domain"
	"github.com/khanzadimahdi/testproject/domain/workload/node"
	"github.com/khanzadimahdi/testproject/domain/workload/stack"
	"github.com/khanzadimahdi/testproject/domain/workload/vm"
	"github.com/khanzadimahdi/testproject/domain/workload/vm/events"
)

// unlisted is a VM its node has not listed for a while: whatever it was, the
// node holds nothing of it any more.
func unlisted(v vm.VM) vm.VM {
	v.LastHeartbeatAt = time.Now().Add(-time.Hour)

	return v
}

func with(v vm.VM, change func(*vm.VM)) vm.VM {
	change(&v)

	return v
}

type transition struct {
	vm       vm.VM
	nodes    []node.Node
	wantErr  error
	state    vm.State
	expected vm.State
	asked    []string
}

func run(t *testing.T, tt transition, act func(*lifecycle.Lifecycle, *vm.VM) error) {
	t.Helper()

	opts := []vmtest.Option{vmtest.WithVMs(tt.vm)}
	if tt.nodes != nil {
		opts = append(opts, vmtest.WithNodes(tt.nodes...))
	}

	w := vmtest.New(opts...)
	v := tt.vm

	err := act(w.Lifecycle, &v)
	if tt.wantErr != nil {
		assert.ErrorIs(t, err, tt.wantErr)
	} else {
		require.NoError(t, err)
	}

	stored, ok := w.VMs.Stored(tt.vm.UUID)
	require.True(t, ok)

	assert.Equal(t, tt.state.String(), stored.CurrentState.String(), "what it is")
	assert.Equal(t, tt.expected.String(), stored.ExpectedState.String(), "what is wanted of it")
	assert.Equal(t, tt.asked, w.Producer.Subjects(), "what its node was asked")
}

func TestLifecycle_Up(t *testing.T) {
	t.Parallel()

	for name, tt := range map[string]transition{
		"a stopped vm its node holds is started": {
			vm:       vmtest.Stopped("01", "owner"),
			state:    vm.Starting,
			expected: vm.Running,
			asked:    []string{events.VMStartRequestedName},
		},
		"one its node no longer holds is made again": {
			vm:       unlisted(with(vmtest.Running("01", "owner"), func(v *vm.VM) { v.CurrentState = vm.Failed })),
			state:    vm.Scheduled,
			expected: vm.Running,
			asked:    []string{events.VMScheduledName},
		},
		"one that is running is where it was asked to be": {
			vm:       vmtest.Running("01", "owner"),
			state:    vm.Running,
			expected: vm.Running,
		},
		"one on no node is placed, and its node asked to make it": {
			vm:       with(vmtest.Stopped("01", "owner"), func(v *vm.VM) { v.NodeName = ""; v.CurrentState = vm.Failed }),
			state:    vm.Scheduled,
			expected: vm.Running,
			asked:    []string{events.VMScheduledName},
		},
		"one on no node that no node has room for is failed and given up on": {
			vm:       with(vmtest.Stopped("01", "owner"), func(v *vm.VM) { v.NodeName = ""; v.Resources.Memory = 128 * vmtest.GiB }),
			wantErr:  vm.ErrNoCapacity,
			state:    vm.Failed,
			expected: vm.Failed,
		},
		"one on its way somewhere is told what is wanted, and asked later": {
			vm:       with(vmtest.Running("01", "owner"), func(v *vm.VM) { v.CurrentState = vm.Stopping; v.ExpectedState = vm.Stopped }),
			state:    vm.Stopping,
			expected: vm.Running,
		},
		"one on a node that has gone quiet is told what is wanted, and asked later": {
			vm:       vmtest.Stopped("01", "owner"),
			nodes:    []node.Node{vmtest.Gone(vmtest.Node)},
			state:    vm.Stopped,
			expected: vm.Running,
		},
		"one on its way out is not brought back": {
			vm:       with(vmtest.Running("01", "owner"), func(v *vm.VM) { v.CurrentState = vm.Deleting; v.ExpectedState = vm.Deleting }),
			wantErr:  lifecycle.ErrBusy,
			state:    vm.Deleting,
			expected: vm.Deleting,
		},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			run(t, tt, func(l *lifecycle.Lifecycle, v *vm.VM) error { return l.Up(context.Background(), v) })
		})
	}
}

func TestLifecycle_Down(t *testing.T) {
	t.Parallel()

	for name, tt := range map[string]transition{
		"a running vm is stopped": {
			vm:       vmtest.Running("01", "owner"),
			state:    vm.Stopping,
			expected: vm.Stopped,
			asked:    []string{events.VMStopRequestedName},
		},
		"a stopped one is stopped already": {
			vm:       vmtest.Stopped("01", "owner"),
			state:    vm.Stopped,
			expected: vm.Stopped,
		},
		"one its node no longer holds has nothing running": {
			vm:       unlisted(vmtest.Running("01", "owner")),
			state:    vm.Stopped,
			expected: vm.Stopped,
		},
		"a failed one stays failed, and is not brought back": {
			vm:       with(vmtest.Running("01", "owner"), func(v *vm.VM) { v.CurrentState = vm.Failed }),
			state:    vm.Failed,
			expected: vm.Stopped,
		},
		"one on its way up is told what is wanted, and asked later": {
			vm:       with(vmtest.Running("01", "owner"), func(v *vm.VM) { v.CurrentState = vm.Starting }),
			state:    vm.Starting,
			expected: vm.Stopped,
		},
		"one on its way out is not asked": {
			vm:       with(vmtest.Running("01", "owner"), func(v *vm.VM) { v.CurrentState = vm.Deleting; v.ExpectedState = vm.Deleting }),
			wantErr:  lifecycle.ErrBusy,
			state:    vm.Deleting,
			expected: vm.Deleting,
		},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			run(t, tt, func(l *lifecycle.Lifecycle, v *vm.VM) error { return l.Down(context.Background(), v) })
		})
	}
}

func TestLifecycle_Restart(t *testing.T) {
	t.Parallel()

	for name, tt := range map[string]transition{
		"a running vm is restarted in place": {
			vm:       vmtest.Running("01", "owner"),
			state:    vm.Restarting,
			expected: vm.Running,
			asked:    []string{events.VMRestartRequestedName},
		},
		"a stopped one is started": {
			vm:       vmtest.Stopped("01", "owner"),
			state:    vm.Starting,
			expected: vm.Running,
			asked:    []string{events.VMStartRequestedName},
		},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			run(t, tt, func(l *lifecycle.Lifecycle, v *vm.VM) error { return l.Restart(context.Background(), v) })
		})
	}
}

func TestLifecycle_Reconfigure(t *testing.T) {
	t.Parallel()

	for name, tt := range map[string]transition{
		"a running vm is given what it now has, restarting it": {
			vm:       vmtest.Running("01", "owner"),
			state:    vm.Restarting,
			expected: vm.Running,
			asked:    []string{events.VMReconfigureRequestedName},
		},
		"a stopped one is given it without being started": {
			vm:       vmtest.Stopped("01", "owner"),
			state:    vm.Stopped,
			expected: vm.Stopped,
			asked:    []string{events.VMReconfigureRequestedName},
		},
		"one its node holds nothing of is made with it next time": {
			vm:       unlisted(vmtest.Stopped("01", "owner")),
			state:    vm.Stopped,
			expected: vm.Stopped,
		},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			run(t, tt, func(l *lifecycle.Lifecycle, v *vm.VM) error { return l.Reconfigure(context.Background(), v) })
		})
	}
}

func TestLifecycle_Remove(t *testing.T) {
	t.Parallel()

	ctx := context.Background()

	t.Run("its node is asked to remove it", func(t *testing.T) {
		t.Parallel()

		run(t, transition{
			vm:       vmtest.Running("01", "owner"),
			state:    vm.Deleting,
			expected: vm.Deleting,
			asked:    []string{events.VMDeleteRequestedName},
		}, func(l *lifecycle.Lifecycle, v *vm.VM) error { return l.Remove(ctx, v) })
	})

	for name, change := range map[string]func(*vm.VM){
		"one on no node is forgotten at once":                    func(v *vm.VM) { v.NodeName = "" },
		"one on a node that has gone quiet is forgotten at once": func(v *vm.VM) { v.NodeName = "workload-orchestrator-09" },
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			v := with(vmtest.Running("01", "owner"), change)

			w := vmtest.New(
				vmtest.WithVMs(v),
				vmtest.WithStacks(
					stack.Stack{UUID: "s1", VMUUID: "01", Slug: "web-a"},
					stack.Stack{UUID: "s2", VMUUID: "02", Slug: "web-b"},
				),
			)

			require.NoError(t, w.Lifecycle.Remove(ctx, &v))

			_, err := w.VMs.GetOne(ctx, "01")
			assert.ErrorIs(t, err, domain.ErrNotExists)

			_, kept := w.Stacks.Stored("s1")
			assert.False(t, kept, "its stacks went with its disk")

			_, kept = w.Stacks.Stored("s2")
			assert.True(t, kept, "another vm's did not")

			assert.Empty(t, w.Producer.Produced())
		})
	}
}

func TestRefused(t *testing.T) {
	t.Parallel()

	refused, err := lifecycle.Refused(lifecycle.ErrBusy)
	require.NoError(t, err)
	assert.Equal(t, domain.ValidationErrors{"vm": "invalid_state_transition"}, refused)

	refused, err = lifecycle.Refused(vm.ErrNoCapacity)
	require.NoError(t, err)
	assert.Equal(t, domain.ValidationErrors{"vm": "no_capacity"}, refused)

	_, err = lifecycle.Refused(domain.ErrNotExists)
	assert.ErrorIs(t, err, domain.ErrNotExists, "what is not a refusal is an error")
}
