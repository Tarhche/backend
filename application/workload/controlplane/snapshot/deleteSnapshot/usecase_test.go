package deleteSnapshot

import (
	"context"
	"errors"
	"log/slog"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/khanzadimahdi/testproject/application/workload/controlplane/snapshot/archive"
	"github.com/khanzadimahdi/testproject/application/workload/controlplane/snapshot/snapshottest"
	"github.com/khanzadimahdi/testproject/application/workload/controlplane/vm/vmtest"
	"github.com/khanzadimahdi/testproject/domain"
	"github.com/khanzadimahdi/testproject/domain/workload/node"
	"github.com/khanzadimahdi/testproject/domain/workload/snapshot"
	"github.com/khanzadimahdi/testproject/infrastructure/translator"
	"github.com/khanzadimahdi/testproject/infrastructure/validator"
)

func useCaseOf(w *vmtest.Workload, bucket snapshot.Store) *UseCase {
	logger := slog.New(slog.DiscardHandler)

	return NewUseCase(w.Snapshots, w.VMs, w.Lifecycle, archive.NewRemover(bucket, logger), validator.New(translator.Codes{}))
}

func TestUseCase_Execute(t *testing.T) {
	t.Parallel()

	ctx := context.Background()

	t.Run("a stored snapshot goes, archive first", func(t *testing.T) {
		t.Parallel()

		w := vmtest.New(vmtest.WithSnapshots(snapshot.Snapshot{UUID: "s1", OwnerUUID: "owner", State: snapshot.Ready}))
		bucket := snapshottest.NewBucket(snapshot.ObjectKey("s1"))

		response, err := useCaseOf(w, bucket).Execute(ctx, &Request{OwnerUUID: "owner", UUID: "s1"})
		require.NoError(t, err)
		assert.False(t, response.Pending)

		_, kept := w.Snapshots.Stored("s1")
		assert.False(t, kept)
		assert.False(t, bucket.Has(snapshot.ObjectKey("s1")))
	})

	t.Run("one whose archive is not there goes all the same", func(t *testing.T) {
		t.Parallel()

		w := vmtest.New(vmtest.WithSnapshots(snapshot.Snapshot{UUID: "s1", State: snapshot.Failed}))

		_, err := useCaseOf(w, snapshottest.NewBucket()).Execute(ctx, &Request{UUID: "s1"})
		require.NoError(t, err)

		_, kept := w.Snapshots.Stored("s1")
		assert.False(t, kept)
	})

	t.Run("one whose archive cannot be taken away stays, going", func(t *testing.T) {
		t.Parallel()

		w := vmtest.New(vmtest.WithSnapshots(snapshot.Snapshot{UUID: "s1", State: snapshot.Ready}))
		bucket := snapshottest.NewBucket(snapshot.ObjectKey("s1"))
		bucket.Fail = errors.New("connection refused")

		_, err := useCaseOf(w, bucket).Execute(ctx, &Request{UUID: "s1"})
		assert.Error(t, err)

		stored, kept := w.Snapshots.Stored("s1")
		require.True(t, kept)
		assert.Equal(t, snapshot.Deleting, stored.State)
	})

	t.Run("one still being taken goes once its node has finished with it", func(t *testing.T) {
		t.Parallel()

		w := vmtest.New(
			vmtest.WithVMs(vmtest.Running("01", "owner")),
			vmtest.WithSnapshots(snapshot.Snapshot{UUID: "s1", VMUUID: "01", State: snapshot.Creating}),
		)
		bucket := snapshottest.NewBucket()

		response, err := useCaseOf(w, bucket).Execute(ctx, &Request{UUID: "s1"})
		require.NoError(t, err)
		assert.True(t, response.Pending)

		stored, kept := w.Snapshots.Stored("s1")
		require.True(t, kept)
		assert.Equal(t, snapshot.Deleting, stored.State)
		assert.Empty(t, bucket.Deleted(), "there is nothing to take away yet")
	})

	t.Run("one being taken on a node that has gone goes at once", func(t *testing.T) {
		t.Parallel()

		w := vmtest.New(
			vmtest.WithNodes(vmtest.Gone(vmtest.Node)),
			vmtest.WithVMs(vmtest.Running("01", "owner")),
			vmtest.WithSnapshots(snapshot.Snapshot{UUID: "s1", VMUUID: "01", State: snapshot.Creating}),
		)

		response, err := useCaseOf(w, snapshottest.NewBucket()).Execute(ctx, &Request{UUID: "s1"})
		require.NoError(t, err)
		assert.False(t, response.Pending)

		_, kept := w.Snapshots.Stored("s1")
		assert.False(t, kept)
	})

	t.Run("somebody else's is not there", func(t *testing.T) {
		t.Parallel()

		w := vmtest.New(vmtest.WithNodes([]node.Node{}...), vmtest.WithSnapshots(snapshot.Snapshot{UUID: "s1", OwnerUUID: "owner", State: snapshot.Ready}))

		_, err := useCaseOf(w, snapshottest.NewBucket()).Execute(ctx, &Request{OwnerUUID: "other", UUID: "s1"})
		assert.ErrorIs(t, err, domain.ErrNotExists)
	})
}
