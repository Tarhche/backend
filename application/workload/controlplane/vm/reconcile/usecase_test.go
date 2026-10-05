package reconcile

import (
	"context"
	"log/slog"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/khanzadimahdi/testproject/application/workload/controlplane/stack/dispatch"
	"github.com/khanzadimahdi/testproject/application/workload/controlplane/vm/lifecycle"
	"github.com/khanzadimahdi/testproject/application/workload/controlplane/vm/vmtest"
	"github.com/khanzadimahdi/testproject/domain/workload/node"
	"github.com/khanzadimahdi/testproject/domain/workload/stack"
	stackEvents "github.com/khanzadimahdi/testproject/domain/workload/stack/events"
	"github.com/khanzadimahdi/testproject/domain/workload/vm"
	"github.com/khanzadimahdi/testproject/domain/workload/vm/events"
)

func useCaseOf(w *vmtest.Workload) *UseCase {
	logger := slog.New(slog.DiscardHandler)

	return NewUseCase(w.VMs, w.Nodes, w.Stacks, w.Lifecycle, w.Commander, dispatch.New(w.Stacks, w.Producer, logger), logger)
}

func TestUseCase_Execute(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	now := time.Now()

	// a VM as it is, changed by each case into what it is meant to show.
	a := func(state vm.State, expected vm.State, change ...func(*vm.VM)) vm.VM {
		v := vmtest.Running("01", "owner")
		v.CurrentState = state
		v.ExpectedState = expected
		v.LastHeartbeatAt = now.Add(-5 * time.Second)
		v.UpdatedAt = now.Add(-time.Hour)

		for _, c := range change {
			c(&v)
		}

		return v
	}

	unlisted := func(v *vm.VM) { v.LastHeartbeatAt = now.Add(-time.Hour) }
	recently := func(v *vm.VM) { v.UpdatedAt = now.Add(-10 * time.Second) }

	for name, tt := range map[string]struct {
		vm       vm.VM
		nodes    []node.Node
		state    vm.State
		expected vm.State
		reason   string
		asked    []string
		gone     bool
	}{
		"a vm running as it was asked is left alone": {
			vm:       a(vm.Running, vm.Running),
			state:    vm.Running,
			expected: vm.Running,
		},
		"one stopped from inside is started again": {
			vm:       a(vm.Stopped, vm.Running),
			state:    vm.Starting,
			expected: vm.Running,
			asked:    []string{events.VMStartRequestedName},
		},
		"one its node no longer lists is made again": {
			vm:       a(vm.Running, vm.Running, unlisted),
			state:    vm.Scheduled,
			expected: vm.Running,
			asked:    []string{events.VMScheduledName},
		},
		"one that fell over a moment ago is left a while": {
			vm:       a(vm.Failed, vm.Running, recently, func(v *vm.VM) { v.Reason = "kernel panic" }),
			state:    vm.Failed,
			expected: vm.Running,
			reason:   "kernel panic",
		},
		"one that fell over a while ago is started again": {
			vm:       a(vm.Failed, vm.Running, func(v *vm.VM) { v.Reason = "kernel panic" }),
			state:    vm.Starting,
			expected: vm.Running,
			asked:    []string{events.VMStartRequestedName},
		},
		"one given up on is left alone": {
			vm:       a(vm.Failed, vm.Failed, func(v *vm.VM) { v.Reason = "pull access denied" }),
			state:    vm.Failed,
			expected: vm.Failed,
			reason:   "pull access denied",
		},
		"one that came back up after it was stopped is stopped again": {
			vm:       a(vm.Running, vm.Stopped),
			state:    vm.Stopping,
			expected: vm.Stopped,
			asked:    []string{events.VMStopRequestedName},
		},
		"one on a node that has gone quiet is lost, and asked nothing": {
			vm:       a(vm.Running, vm.Running),
			nodes:    []node.Node{vmtest.Gone(vmtest.Node)},
			state:    vm.Failed,
			expected: vm.Running,
			reason:   lifecycle.ReasonNodeLost,
		},
		"a stopped one on a node that has gone quiet is left stopped": {
			vm:       a(vm.Stopped, vm.Stopped),
			nodes:    []node.Node{vmtest.Gone(vmtest.Node)},
			state:    vm.Stopped,
			expected: vm.Stopped,
		},
		"one that has outlived its lifetime is deleted": {
			vm:       a(vm.Running, vm.Running, func(v *vm.VM) { v.Lifetime = time.Hour; v.ExpiresAt = now.Add(-time.Second) }),
			state:    vm.Deleting,
			expected: vm.Deleting,
			asked:    []string{events.VMDeleteRequestedName},
		},
		"one whose deletion was asked a moment ago is waited for": {
			vm:       a(vm.Deleting, vm.Deleting, recently),
			state:    vm.Deleting,
			expected: vm.Deleting,
		},
		"one whose deletion was never confirmed is asked for again": {
			vm:       a(vm.Deleting, vm.Deleting),
			state:    vm.Deleting,
			expected: vm.Deleting,
			asked:    []string{events.VMDeleteRequestedName},
		},
		"one on its way out whose node has gone is forgotten": {
			vm:    a(vm.Deleting, vm.Deleting),
			nodes: []node.Node{vmtest.Gone(vmtest.Node)},
			gone:  true,
		},
		"one being made is given time": {
			vm:       a(vm.Scheduled, vm.Running, unlisted, func(v *vm.VM) { v.UpdatedAt = now.Add(-time.Minute) }),
			state:    vm.Scheduled,
			expected: vm.Running,
		},
		"one being made for longer than it takes is asked for again": {
			vm:       a(vm.Scheduled, vm.Running, unlisted),
			state:    vm.Scheduled,
			expected: vm.Running,
			asked:    []string{events.VMScheduledName},
		},
		"one that never stopped is asked to again": {
			vm:       a(vm.Stopping, vm.Stopped),
			state:    vm.Stopping,
			expected: vm.Stopped,
			asked:    []string{events.VMStopRequestedName},
		},
		"one being stopped that its node no longer holds has stopped": {
			vm:       a(vm.Stopping, vm.Stopped, unlisted),
			state:    vm.Stopped,
			expected: vm.Stopped,
		},
		"one whose restore was never reported is asked to restore again": {
			vm:       a(vm.Restoring, vm.Running, func(v *vm.VM) { v.RestoreFrom = "snapshot-uuid" }),
			state:    vm.Restoring,
			expected: vm.Running,
			asked:    []string{events.VMRestoreRequestedName},
		},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			opts := []vmtest.Option{vmtest.WithVMs(tt.vm)}
			if tt.nodes != nil {
				opts = append(opts, vmtest.WithNodes(tt.nodes...))
			}

			w := vmtest.New(opts...)

			require.NoError(t, useCaseOf(w).Execute(ctx))

			stored, kept := w.VMs.Stored("01")
			if tt.gone {
				assert.False(t, kept)

				return
			}

			require.True(t, kept)
			assert.Equal(t, tt.state.String(), stored.CurrentState.String(), "what it is")
			assert.Equal(t, tt.expected.String(), stored.ExpectedState.String(), "what is wanted of it")
			assert.Equal(t, tt.reason, stored.Reason)
			assert.Equal(t, tt.asked, w.Producer.Subjects(), "what its node was asked")
		})
	}

	t.Run("a stack waiting for a vm that came up is sent", func(t *testing.T) {
		t.Parallel()

		w := vmtest.New(
			vmtest.WithVMs(vmtest.Docker("01", "owner")),
			vmtest.WithStacks(stack.Stack{UUID: "s1", VMUUID: "01", Slug: "web-abcde", State: stack.Deploying, ExpectedState: stack.Running, Reason: dispatch.ReasonWaitingForVM}),
		)

		require.NoError(t, useCaseOf(w).Execute(ctx))

		var asked stackEvents.StackRequested
		require.True(t, w.Producer.Last(stackEvents.StackRequestedName, &asked))
		assert.Equal(t, "s1", asked.StackUUID)
		assert.Equal(t, stack.ActionUp, asked.Action)
	})

	t.Run("one waiting for a vm that is not coming up fails", func(t *testing.T) {
		t.Parallel()

		givenUp := vmtest.Docker("01", "owner")
		givenUp.CurrentState = vm.Failed
		givenUp.ExpectedState = vm.Failed

		w := vmtest.New(
			vmtest.WithVMs(givenUp),
			vmtest.WithStacks(stack.Stack{UUID: "s1", VMUUID: "01", Slug: "web-abcde", State: stack.Deploying, ExpectedState: stack.Running, Reason: dispatch.ReasonWaitingForVM}),
		)

		require.NoError(t, useCaseOf(w).Execute(ctx))

		stored, _ := w.Stacks.Stored("s1")
		assert.Equal(t, stack.Failed, stored.State)
		assert.Equal(t, ReasonNotRunning, stored.Reason)
	})

	t.Run("every vm is looked at, a batch at a time", func(t *testing.T) {
		t.Parallel()

		var stopped []vm.VM
		for i := range 45 {
			v := vmtest.Stopped(string(rune('a'+i/26))+string(rune('a'+i%26)), "owner")
			v.ExpectedState = vm.Running
			stopped = append(stopped, v)
		}

		w := vmtest.New(vmtest.WithVMs(stopped...))

		require.NoError(t, useCaseOf(w).Execute(ctx))

		assert.Len(t, w.Producer.Produced(), 45)
	})
}
