//go:build integration

package stacks

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"

	"github.com/khanzadimahdi/testproject/domain"
	"github.com/khanzadimahdi/testproject/domain/workload/snapshot"
	"github.com/khanzadimahdi/testproject/domain/workload/stack"
	"github.com/khanzadimahdi/testproject/infrastructure/repository/mongodb/migrations"
	"github.com/khanzadimahdi/testproject/infrastructure/repository/mongodb/workload/snapshots"
)

// database is a fresh database on the server MONGO_TEST_URI names, with the
// migrations applied, or a skipped test when none is named.
func database(t *testing.T) *mongo.Database {
	t.Helper()

	uri := os.Getenv("MONGO_TEST_URI")
	if len(uri) == 0 {
		t.Skip("MONGO_TEST_URI names no database to test against")
	}

	client, err := mongo.Connect(options.Client().ApplyURI(uri))
	require.NoError(t, err)

	db := client.Database("stacks_test_" + time.Now().Format("150405000000"))
	t.Cleanup(func() {
		_ = db.Drop(context.Background())
		_ = client.Disconnect(context.Background())
	})

	_, err = migrations.NewMigrator(db, migrations.All()...).Migrate(context.Background())
	require.NoError(t, err)

	return db
}

func TestStacksRepository(t *testing.T) {
	ctx := context.Background()
	db := database(t)
	repository := NewRepository(db)

	web := stack.Stack{
		Name:          "web",
		OwnerUUID:     "owner",
		VMUUID:        "vm-uuid",
		Slug:          "web-abcde",
		Compose:       "services:\n  web:\n    image: nginx\n",
		ExpectedState: stack.Running,
		State:         stack.Deploying,
		Reason:        "waiting",
		UpdatedAt:     time.Now(),
	}

	uuid, err := repository.Save(ctx, &web)
	require.NoError(t, err)

	t.Run("a result read before a newer request is refused", func(t *testing.T) {
		stored, err := repository.GetOne(ctx, uuid)
		require.NoError(t, err)

		result := stored

		stored.ExpectedState = stack.Stopped
		stored.State = stack.Stopping
		stored.Reason = ""
		stored.UpdatedAt = stored.UpdatedAt.Add(time.Second)
		_, err = repository.Save(ctx, &stored)
		require.NoError(t, err)

		result.State = stack.Running
		_, err = repository.Save(ctx, &result)
		assert.ErrorIs(t, err, ErrStale)

		again, err := repository.GetOneBySlug(ctx, "web-abcde")
		require.NoError(t, err)
		assert.Equal(t, stack.Stopping, again.State)
		assert.Empty(t, again.Reason, "a cleared reason stays cleared")
	})

	t.Run("a slug is one stack's", func(t *testing.T) {
		_, err := repository.Save(ctx, &stack.Stack{Slug: "web-abcde"})
		assert.ErrorIs(t, err, domain.ErrAlreadyExists)
	})

	t.Run("listings", func(t *testing.T) {
		byVM, err := repository.GetAllByVM(ctx, "vm-uuid")
		require.NoError(t, err)
		assert.Len(t, byVM, 1)

		_, err = repository.GetOneByOwner(ctx, "somebody-else", uuid)
		assert.ErrorIs(t, err, domain.ErrNotExists)
	})

	t.Run("snapshots read back as they were written", func(t *testing.T) {
		snapshotsRepository := snapshots.NewRepository(db)

		taken := snapshot.Snapshot{Name: "before", OwnerUUID: "owner", VMUUID: "vm-uuid", State: snapshot.Creating, Disk: 20 << 30}
		id, err := snapshotsRepository.Save(ctx, &taken)
		require.NoError(t, err)

		stored, err := snapshotsRepository.GetOneByOwner(ctx, "owner", id)
		require.NoError(t, err)
		assert.Equal(t, uint64(20<<30), stored.Disk)

		byVM, err := snapshotsRepository.GetAllByVM(ctx, "vm-uuid")
		require.NoError(t, err)
		assert.Len(t, byVM, 1)

		count, err := snapshotsRepository.CountByOwner(ctx, "owner")
		require.NoError(t, err)
		assert.Equal(t, uint(1), count)
	})
}
