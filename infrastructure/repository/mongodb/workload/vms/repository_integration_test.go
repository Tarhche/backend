//go:build integration

package vms

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
	"github.com/khanzadimahdi/testproject/domain/workload/port"
	"github.com/khanzadimahdi/testproject/domain/workload/vm"
	"github.com/khanzadimahdi/testproject/infrastructure/repository/mongodb/migrations"
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

	db := client.Database("vms_test_" + time.Now().Format("150405000000"))
	t.Cleanup(func() {
		_ = db.Drop(context.Background())
		_ = client.Disconnect(context.Background())
	})

	_, err = migrations.NewMigrator(db, migrations.All()...).Migrate(context.Background())
	require.NoError(t, err)

	return db
}

func TestVMsRepository(t *testing.T) {
	ctx := context.Background()
	repository := NewRepository(database(t))

	asked := time.Now().Truncate(time.Millisecond)

	web := vm.VM{
		Name:           "web",
		Slug:           "web-abcde",
		OwnerUUID:      "owner",
		Kind:           vm.KindDocker,
		Image:          "docker:29-dind",
		Resources:      vm.Resources{CPUs: 2, Memory: 2 << 30, Disk: 20 << 30},
		Ports:          []port.Port{80, 443},
		Network:        vm.Network{Ingress: vm.AccessAllow, Egress: vm.AccessDeny},
		PersistentDisk: true,
		Lifetime:       time.Hour,
		ExpiresAt:      asked.Add(time.Hour),
		CurrentState:   vm.Scheduled,
		ExpectedState:  vm.Running,
		Reason:         "pending",
		NodeName:       "workload-orchestrator-01",
		RestoreFrom:    "snapshot-uuid",
		UpdatedAt:      asked,
	}

	uuid, err := repository.Save(ctx, &web)
	require.NoError(t, err)

	t.Run("it reads back as it was written", func(t *testing.T) {
		stored, err := repository.GetOne(ctx, uuid)
		require.NoError(t, err)

		assert.Equal(t, web.Slug, stored.Slug)
		assert.Equal(t, web.Resources, stored.Resources)
		assert.Equal(t, web.Ports, stored.Ports)
		assert.Equal(t, web.Network, stored.Network)
		assert.Equal(t, web.Lifetime, stored.Lifetime)
		assert.True(t, web.ExpiresAt.Equal(stored.ExpiresAt))
		assert.Equal(t, vm.Scheduled, stored.CurrentState)
		assert.Equal(t, "snapshot-uuid", stored.RestoreFrom)
	})

	t.Run("what was cleared stays cleared", func(t *testing.T) {
		stored, err := repository.GetOne(ctx, uuid)
		require.NoError(t, err)

		stored.Reason = ""
		stored.RestoreFrom = ""
		stored.ExpiresAt = time.Time{}
		stored.UpdatedAt = stored.UpdatedAt.Add(time.Second)

		_, err = repository.Save(ctx, &stored)
		require.NoError(t, err)

		again, err := repository.GetOne(ctx, uuid)
		require.NoError(t, err)
		assert.Empty(t, again.Reason)
		assert.Empty(t, again.RestoreFrom)
		assert.True(t, again.ExpiresAt.IsZero())
	})

	t.Run("a copy read before a newer request is refused", func(t *testing.T) {
		stored, err := repository.GetOne(ctx, uuid)
		require.NoError(t, err)

		report := stored

		stored.ExpectedState = vm.Stopped
		stored.UpdatedAt = stored.UpdatedAt.Add(time.Second)
		_, err = repository.Save(ctx, &stored)
		require.NoError(t, err)

		report.CurrentState = vm.Running
		_, err = repository.Save(ctx, &report)
		assert.ErrorIs(t, err, ErrStale)

		again, err := repository.GetOne(ctx, uuid)
		require.NoError(t, err)
		assert.Equal(t, vm.Stopped, again.ExpectedState, "the request stands")
	})

	t.Run("a report that knows of every request is written", func(t *testing.T) {
		stored, err := repository.GetOne(ctx, uuid)
		require.NoError(t, err)

		stored.CurrentState = vm.Running
		_, err = repository.Save(ctx, &stored)
		require.NoError(t, err)
	})

	t.Run("a slug is one vm's", func(t *testing.T) {
		_, err := repository.Save(ctx, &vm.VM{Name: "web", Slug: "web-abcde", OwnerUUID: "other"})
		assert.ErrorIs(t, err, domain.ErrAlreadyExists)
	})

	t.Run("listings", func(t *testing.T) {
		other := vm.VM{Name: "box", Slug: "box-fghij", OwnerUUID: "owner", Kind: vm.KindMachine, NodeName: "workload-orchestrator-02"}
		_, err := repository.Save(ctx, &other)
		require.NoError(t, err)

		owned, err := repository.GetAllByOwner(ctx, "owner", 0, 10)
		require.NoError(t, err)
		require.Len(t, owned, 2)
		assert.Equal(t, other.UUID, owned[0].UUID, "newest first")

		docker, err := repository.GetAllByOwnerAndKind(ctx, "owner", vm.KindDocker)
		require.NoError(t, err)
		require.Len(t, docker, 1)

		held, err := repository.GetAllByNode(ctx, "workload-orchestrator-02")
		require.NoError(t, err)
		require.Len(t, held, 1)

		count, err := repository.Count(ctx)
		require.NoError(t, err)
		assert.Equal(t, uint(2), count)

		require.NoError(t, repository.Delete(ctx, other.UUID))
		_, err = repository.GetOne(ctx, other.UUID)
		assert.ErrorIs(t, err, domain.ErrNotExists)
	})
}
