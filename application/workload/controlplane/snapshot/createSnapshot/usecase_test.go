package createSnapshot

import (
	"context"
	"encoding/json"
	"log/slog"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/khanzadimahdi/testproject/application/workload/controlplane/snapshot/archive"
	"github.com/khanzadimahdi/testproject/application/workload/controlplane/snapshot/snapshottest"
	"github.com/khanzadimahdi/testproject/application/workload/controlplane/vm/vmtest"
	"github.com/khanzadimahdi/testproject/domain"
	"github.com/khanzadimahdi/testproject/domain/workload/node"
	"github.com/khanzadimahdi/testproject/domain/workload/snapshot"
	"github.com/khanzadimahdi/testproject/domain/workload/snapshot/events"
	"github.com/khanzadimahdi/testproject/domain/workload/vm"
	"github.com/khanzadimahdi/testproject/infrastructure/translator"
	"github.com/khanzadimahdi/testproject/infrastructure/validator"
)

func useCaseOf(w *vmtest.Workload, userMax uint) *UseCase {
	return NewUseCase(w.VMs, w.Snapshots, w.Lifecycle, w.Producer, validator.New(translator.Codes{}), userMax)
}

func TestUseCase_Execute(t *testing.T) {
	t.Parallel()

	ctx := context.Background()

	t.Run("the node holding the vm is asked to take it", func(t *testing.T) {
		t.Parallel()

		w := vmtest.New(vmtest.WithVMs(vmtest.Stopped("01", "owner")))

		response, err := useCaseOf(w, 10).Execute(ctx, &Request{OwnerUUID: "owner", VMUUID: "01", Name: " before the upgrade "})
		require.NoError(t, err)
		require.Empty(t, response.ValidationErrors)
		require.NotNil(t, response.Snapshot)

		stored, ok := w.Snapshots.Stored(response.Snapshot.UUID)
		require.True(t, ok)
		assert.Equal(t, "before the upgrade", stored.Name)
		assert.Equal(t, "owner", stored.OwnerUUID)
		assert.Equal(t, "01", stored.VMUUID)
		assert.Equal(t, "box", stored.VMName)
		assert.Equal(t, vm.KindMachine, stored.Kind)
		assert.Equal(t, uint64(10*vmtest.GiB), stored.Disk)
		assert.Equal(t, snapshot.Creating, stored.State)

		var asked events.SnapshotRequested
		require.True(t, w.Producer.Last(events.SnapshotRequestedName, &asked))
		assert.Equal(t, events.SnapshotRequested{SnapshotUUID: stored.UUID, VMUUID: "01", NodeName: vmtest.Node}, asked)
	})

	for name, tt := range map[string]struct {
		vm      vm.VM
		nodes   []node.Node
		kept    []snapshot.Snapshot
		request Request
		want    domain.ValidationErrors
	}{
		"one with no name": {
			vm:      vmtest.Running("01", "owner"),
			request: Request{VMUUID: "01"},
			want:    domain.ValidationErrors{"name": "required_field"},
		},
		"one of a vm on its way somewhere": {
			vm:      func() vm.VM { v := vmtest.Running("01", "owner"); v.CurrentState = vm.Starting; return v }(),
			request: Request{VMUUID: "01", Name: "now"},
			want:    domain.ValidationErrors{"vm": "invalid_state_transition"},
		},
		"one of a vm whose node has gone quiet": {
			vm:      vmtest.Stopped("01", "owner"),
			nodes:   []node.Node{vmtest.Gone(vmtest.Node)},
			request: Request{VMUUID: "01", Name: "now"},
			want:    domain.ValidationErrors{"vm": "invalid_state_transition"},
		},
		"more than one person may keep": {
			vm:      vmtest.Running("01", "owner"),
			kept:    []snapshot.Snapshot{{UUID: "a", OwnerUUID: "owner"}, {UUID: "b", OwnerUUID: "owner"}},
			request: Request{VMUUID: "01", Name: "one too many"},
			want:    domain.ValidationErrors{"snapshots": "quota_exceeded"},
		},
	} {
		t.Run("refused: "+name, func(t *testing.T) {
			t.Parallel()

			opts := []vmtest.Option{vmtest.WithVMs(tt.vm), vmtest.WithSnapshots(tt.kept...)}
			if tt.nodes != nil {
				opts = append(opts, vmtest.WithNodes(tt.nodes...))
			}

			w := vmtest.New(opts...)

			response, err := useCaseOf(w, 2).Execute(ctx, &tt.request)
			require.NoError(t, err)
			assert.Equal(t, tt.want, response.ValidationErrors)
			assert.Empty(t, w.Producer.Messages())
		})
	}

	t.Run("somebody else's vm is not there", func(t *testing.T) {
		t.Parallel()

		w := vmtest.New(vmtest.WithVMs(vmtest.Running("01", "owner")))

		_, err := useCaseOf(w, 10).Execute(ctx, &Request{OwnerUUID: "other", VMUUID: "01", Name: "mine"})
		assert.ErrorIs(t, err, domain.ErrNotExists)
	})
}

func message(t *testing.T, value any) []byte {
	t.Helper()

	payload, err := json.Marshal(value)
	require.NoError(t, err)

	return payload
}

func TestSnapshotCompleted_Handle(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	logger := slog.New(slog.DiscardHandler)
	at := time.Date(2026, 10, 4, 12, 0, 0, 0, time.UTC)

	completed := events.SnapshotCompleted{SnapshotUUID: "s1", Size: 1 << 30, Engine: "microsandbox/0.7.6", Disk: 12 * vmtest.GiB, At: at}

	t.Run("a snapshot being taken is ready once stored", func(t *testing.T) {
		t.Parallel()

		w := vmtest.New(vmtest.WithSnapshots(snapshot.Snapshot{UUID: "s1", State: snapshot.Creating, Disk: 10 * vmtest.GiB}))
		bucket := snapshottest.NewBucket(snapshot.ObjectKey("s1"))

		require.NoError(t, NewSnapshotCompleted(w.Snapshots, archive.NewRemover(bucket, logger), logger).Handle(ctx, message(t, completed)))

		stored, _ := w.Snapshots.Stored("s1")
		assert.Equal(t, snapshot.Ready, stored.State)
		assert.Equal(t, int64(1<<30), stored.Size)
		assert.Equal(t, "microsandbox/0.7.6", stored.Engine)
		assert.Equal(t, uint64(12*vmtest.GiB), stored.Disk)
		assert.True(t, at.Equal(stored.CompletedAt))
		assert.True(t, bucket.Has(snapshot.ObjectKey("s1")))
	})

	t.Run("one deleted while it was taken goes now, archive and all", func(t *testing.T) {
		t.Parallel()

		w := vmtest.New(vmtest.WithSnapshots(snapshot.Snapshot{UUID: "s1", State: snapshot.Deleting}))
		bucket := snapshottest.NewBucket(snapshot.ObjectKey("s1"))

		require.NoError(t, NewSnapshotCompleted(w.Snapshots, archive.NewRemover(bucket, logger), logger).Handle(ctx, message(t, completed)))

		_, kept := w.Snapshots.Stored("s1")
		assert.False(t, kept)
		assert.False(t, bucket.Has(snapshot.ObjectKey("s1")))
	})

	t.Run("one nobody has a record of has its archive taken away", func(t *testing.T) {
		t.Parallel()

		w := vmtest.New()
		bucket := snapshottest.NewBucket(snapshot.ObjectKey("s1"))

		require.NoError(t, NewSnapshotCompleted(w.Snapshots, archive.NewRemover(bucket, logger), logger).Handle(ctx, message(t, completed)))

		assert.False(t, bucket.Has(snapshot.ObjectKey("s1")))
	})

	t.Run("what will never be handled is not handed back", func(t *testing.T) {
		t.Parallel()

		w := vmtest.New()

		assert.NoError(t, NewSnapshotCompleted(w.Snapshots, archive.NewRemover(nil, logger), logger).Handle(ctx, []byte("{")))
	})
}

func TestSnapshotFailed_Handle(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	logger := slog.New(slog.DiscardHandler)

	t.Run("a snapshot that could not be taken is failed, saying why", func(t *testing.T) {
		t.Parallel()

		w := vmtest.New(vmtest.WithSnapshots(snapshot.Snapshot{UUID: "s1", State: snapshot.Creating}))

		require.NoError(t, NewSnapshotFailed(w.Snapshots, archive.NewRemover(nil, logger), logger).Handle(ctx, message(t, events.SnapshotFailed{SnapshotUUID: "s1", Reason: "no space left on device"})))

		stored, _ := w.Snapshots.Stored("s1")
		assert.Equal(t, snapshot.Failed, stored.State)
		assert.Equal(t, "no space left on device", stored.Reason)
		assert.False(t, stored.CompletedAt.IsZero())
	})

	t.Run("one deleted while it was taken goes now", func(t *testing.T) {
		t.Parallel()

		w := vmtest.New(vmtest.WithSnapshots(snapshot.Snapshot{UUID: "s1", State: snapshot.Deleting}))
		bucket := snapshottest.NewBucket()

		require.NoError(t, NewSnapshotFailed(w.Snapshots, archive.NewRemover(bucket, logger), logger).Handle(ctx, message(t, events.SnapshotFailed{SnapshotUUID: "s1"})))

		_, kept := w.Snapshots.Stored("s1")
		assert.False(t, kept)
		assert.Equal(t, []string{snapshot.ObjectKey("s1")}, bucket.Deleted(), "whatever is left of it goes with it")
	})
}
