package restoreVM

import (
	"context"
	"encoding/json"
	"log/slog"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/khanzadimahdi/testproject/application/workload/controlplane/vm/coderunner"
	"github.com/khanzadimahdi/testproject/application/workload/controlplane/vm/vmtest"
	"github.com/khanzadimahdi/testproject/domain"
	"github.com/khanzadimahdi/testproject/domain/workload/node"
	"github.com/khanzadimahdi/testproject/domain/workload/snapshot"
	"github.com/khanzadimahdi/testproject/domain/workload/vm"
	"github.com/khanzadimahdi/testproject/domain/workload/vm/events"
	"github.com/khanzadimahdi/testproject/infrastructure/translator"
	"github.com/khanzadimahdi/testproject/infrastructure/validator"
)

func ready(owner string) snapshot.Snapshot {
	return snapshot.Snapshot{
		UUID:      "snapshot-uuid",
		OwnerUUID: owner,
		Kind:      vm.KindMachine,
		Disk:      5 * vmtest.GiB,
		Engine:    "microsandbox/0.7.6",
		State:     snapshot.Ready,
	}
}

func useCaseOf(w *vmtest.Workload) *UseCase {
	return NewUseCase(w.VMs, w.Runs, w.Snapshots, w.Nodes, w.Lifecycle, w.Commander, validator.New(translator.Codes{}))
}

func TestUseCase_Execute(t *testing.T) {
	t.Parallel()

	ctx := context.Background()

	t.Run("its node is asked to replace its disk", func(t *testing.T) {
		t.Parallel()

		w := vmtest.New(vmtest.WithVMs(vmtest.Running("01", "owner")), vmtest.WithSnapshots(ready("owner")))

		response, err := useCaseOf(w).Execute(ctx, &Request{OwnerUUID: "owner", UUID: "01", SnapshotUUID: "snapshot-uuid"})
		require.NoError(t, err)
		require.Empty(t, response.ValidationErrors)

		stored, _ := w.VMs.Stored("01")
		assert.Equal(t, vm.Restoring, stored.CurrentState)
		assert.Equal(t, "snapshot-uuid", stored.RestoreFrom)

		var asked events.VMRestoreRequested
		require.True(t, w.Producer.Last(events.VMRestoreRequestedName, &asked))
		assert.Equal(t, "snapshot-uuid", asked.SnapshotUUID)
		assert.Equal(t, vmtest.Node, asked.NodeName)
		assert.Equal(t, "01", asked.Spec.ID)
	})

	for name, tt := range map[string]struct {
		vm       vm.VM
		snapshot snapshot.Snapshot
		nodes    []node.Node
		want     domain.ValidationErrors
	}{
		"somebody else's snapshot": {
			vm:       vmtest.Running("01", "owner"),
			snapshot: ready("other"),
			want:     domain.ValidationErrors{"snapshot_uuid": "not_found"},
		},
		"one not stored yet": {
			vm:       vmtest.Running("01", "owner"),
			snapshot: func() snapshot.Snapshot { s := ready("owner"); s.State = snapshot.Creating; return s }(),
			want:     domain.ValidationErrors{"snapshot_uuid": "snapshot_not_ready"},
		},
		"one of another kind": {
			vm:       vmtest.Running("01", "owner"),
			snapshot: func() snapshot.Snapshot { s := ready("owner"); s.Kind = vm.KindDocker; return s }(),
			want:     domain.ValidationErrors{"snapshot_uuid": "kind_mismatch"},
		},
		"one with a larger disk than the vm's": {
			vm:       vmtest.Running("01", "owner"),
			snapshot: func() snapshot.Snapshot { s := ready("owner"); s.Disk = 20 * vmtest.GiB; return s }(),
			want:     domain.ValidationErrors{"snapshot_uuid": "disk_too_small"},
		},
		"one another engine wrote": {
			vm:       vmtest.Running("01", "owner"),
			snapshot: func() snapshot.Snapshot { s := ready("owner"); s.Engine = "firecracker/1.9"; return s }(),
			want:     domain.ValidationErrors{"snapshot_uuid": "engine_mismatch"},
		},
		"a vm on its way somewhere": {
			vm:       func() vm.VM { v := vmtest.Running("01", "owner"); v.CurrentState = vm.Starting; return v }(),
			snapshot: ready("owner"),
			want:     domain.ValidationErrors{"vm": "invalid_state_transition"},
		},
		"a vm on a node that has gone quiet": {
			vm:       vmtest.Running("01", "owner"),
			snapshot: ready("owner"),
			nodes:    []node.Node{vmtest.Gone(vmtest.Node)},
			want:     domain.ValidationErrors{"vm": "invalid_state_transition"},
		},
	} {
		t.Run("refused: "+name, func(t *testing.T) {
			t.Parallel()

			opts := []vmtest.Option{vmtest.WithVMs(tt.vm), vmtest.WithSnapshots(tt.snapshot)}
			if tt.nodes != nil {
				opts = append(opts, vmtest.WithNodes(tt.nodes...))
			}

			w := vmtest.New(opts...)

			response, err := useCaseOf(w).Execute(ctx, &Request{UUID: "01", SnapshotUUID: "snapshot-uuid"})
			require.NoError(t, err)
			assert.Equal(t, tt.want, response.ValidationErrors)
			assert.Empty(t, w.Producer.Messages())
		})
	}
}

func restoredEvent(t *testing.T, snapshotUUID string) []byte {
	t.Helper()

	payload, err := json.Marshal(events.VMRestored{VMUUID: "01", NodeName: vmtest.Node, SnapshotUUID: snapshotUUID})
	require.NoError(t, err)

	return payload
}

func TestVMRestored_Handle(t *testing.T) {
	t.Parallel()

	ctx := context.Background()

	restoring := func(expected vm.State) vm.VM {
		v := vmtest.Running("01", "owner")
		v.CurrentState = vm.Restoring
		v.ExpectedState = expected
		v.RestoreFrom = "snapshot-uuid"

		return v
	}

	t.Run("a vm wanted running is starting once restored", func(t *testing.T) {
		t.Parallel()

		w := vmtest.New(vmtest.WithVMs(restoring(vm.Running)))

		require.NoError(t, NewVMRestored(w.VMs, w.Lifecycle, w.Commander, slog.New(slog.DiscardHandler)).Handle(ctx, restoredEvent(t, "snapshot-uuid")))

		stored, _ := w.VMs.Stored("01")
		assert.Equal(t, vm.Starting, stored.CurrentState)
		assert.Empty(t, stored.RestoreFrom)
		assert.Empty(t, w.Producer.Messages())
		assert.Equal(t, []string{"vm/01"}, w.Children.RestoredParents(), "what lives on its disk is what the restored disk holds")
	})

	t.Run("one wanted stopped is stopped again", func(t *testing.T) {
		t.Parallel()

		w := vmtest.New(vmtest.WithVMs(restoring(vm.Stopped)))

		require.NoError(t, NewVMRestored(w.VMs, w.Lifecycle, w.Commander, slog.New(slog.DiscardHandler)).Handle(ctx, restoredEvent(t, "snapshot-uuid")))

		stored, _ := w.VMs.Stored("01")
		assert.Equal(t, vm.Stopping, stored.CurrentState)
		assert.Equal(t, []string{events.VMStopRequestedName}, w.Producer.Subjects())
	})

	t.Run("a restore the vm is no longer waiting on is not taken for its own", func(t *testing.T) {
		t.Parallel()

		w := vmtest.New(vmtest.WithVMs(restoring(vm.Running)))

		require.NoError(t, NewVMRestored(w.VMs, w.Lifecycle, w.Commander, slog.New(slog.DiscardHandler)).Handle(ctx, restoredEvent(t, "another-snapshot")))

		stored, _ := w.VMs.Stored("01")
		assert.Equal(t, vm.Restoring, stored.CurrentState)
		assert.Empty(t, w.Children.RestoredParents())
	})

	t.Run("a vm made from a snapshot is made from it no longer, once it has been", func(t *testing.T) {
		t.Parallel()

		made := restoring(vm.Running)
		made.CurrentState = vm.Scheduled

		w := vmtest.New(vmtest.WithVMs(made))

		require.NoError(t, NewVMRestored(w.VMs, w.Lifecycle, w.Commander, slog.New(slog.DiscardHandler)).Handle(ctx, restoredEvent(t, "snapshot-uuid")))

		stored, _ := w.VMs.Stored("01")
		assert.Equal(t, vm.Scheduled, stored.CurrentState, "its node reports when it is up")
		assert.Empty(t, stored.RestoreFrom)
		assert.Empty(t, w.Producer.Messages())
		assert.Empty(t, w.Children.RestoredParents(), "a vm made anew has nothing living in it yet")
	})

	t.Run("what will never be handled is not handed back", func(t *testing.T) {
		t.Parallel()

		w := vmtest.New()

		assert.NoError(t, NewVMRestored(w.VMs, w.Lifecycle, w.Commander, slog.New(slog.DiscardHandler)).Handle(ctx, []byte("nope")))
	})
}

// TestUseCase_Execute_run holds a run of the code runner's to being refused
// what only a VM somebody asked for can be asked, by whoever may see it, and to
// being nobody's own.
func TestUseCase_Execute_run(t *testing.T) {
	t.Parallel()

	ctx := context.Background()

	t.Run("anybody's is refused", func(t *testing.T) {
		t.Parallel()

		w := vmtest.New(vmtest.WithTasks(vmtest.Run("run")))

		response, err := useCaseOf(w).Execute(ctx, &Request{UUID: "run", SnapshotUUID: "snapshot-uuid"})
		require.NoError(t, err)
		assert.Equal(t, domain.ValidationErrors{"vm": coderunner.CodeRefused}, response.ValidationErrors)
		assert.Empty(t, w.Producer.Messages())

		stored, _ := w.Tasks.Stored("run")
		assert.Equal(t, vmtest.Run("run").CurrentState, stored.CurrentState)
	})

	t.Run("one's own is not there", func(t *testing.T) {
		t.Parallel()

		w := vmtest.New(vmtest.WithTasks(vmtest.Run("run")))

		_, err := useCaseOf(w).Execute(ctx, &Request{OwnerUUID: "owner", UUID: "run", SnapshotUUID: "snapshot-uuid"})
		assert.ErrorIs(t, err, domain.ErrNotExists)
		assert.Empty(t, w.Producer.Messages())
	})
}
