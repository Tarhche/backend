//go:build integration

package migrations

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"

	"github.com/khanzadimahdi/testproject/domain/workload/kind"
	snapshotKind "github.com/khanzadimahdi/testproject/domain/workload/kinds/snapshot"
	"github.com/khanzadimahdi/testproject/domain/workload/resource"
	"github.com/khanzadimahdi/testproject/domain/workload/vm"
	"github.com/khanzadimahdi/testproject/infrastructure/repository/mongodb/workload/resources"
)

// TestSnapshotsAsManifests converts snapshots kept as they were before a
// snapshot was a kind, on the server MONGO_TEST_URI names, and reads them back
// as the snapshot kind's repository does.
func TestSnapshotsAsManifests(t *testing.T) {
	uri := os.Getenv("MONGO_TEST_URI")
	if len(uri) == 0 {
		t.Skip("MONGO_TEST_URI names no database to test against")
	}

	ctx := context.Background()

	client, err := mongo.Connect(options.Client().ApplyURI(uri))
	require.NoError(t, err)

	database := client.Database(fmt.Sprintf("migrations_test_%s", time.Now().Format("150405000000")))
	t.Cleanup(func() {
		_ = database.Drop(context.Background())
		_ = client.Disconnect(context.Background())
	})

	// what the snapshots were kept as, and indexed by, before.
	require.NoError(t, indexWorkloadVMs.Up(ctx, database))

	created := time.Date(2026, 10, 5, 12, 0, 0, 0, time.UTC)

	_, err = database.Collection("snapshots").InsertMany(ctx, []any{
		bson.D{
			{Key: "_id", Value: "ready-uuid"}, {Key: "name", Value: "before the upgrade"}, {Key: "owner_uuid", Value: "owner-uuid"},
			{Key: "vm_uuid", Value: "vm-uuid"}, {Key: "vm_name", Value: "box"}, {Key: "kind", Value: "machine"},
			{Key: "image", Value: "ubuntu:24.04"}, {Key: "disk", Value: int64(10 << 30)}, {Key: "engine", Value: "microsandbox/0.7.6"},
			{Key: "size", Value: int64(1 << 30)}, {Key: "state", Value: 2}, {Key: "reason", Value: ""},
			{Key: "created_at", Value: created}, {Key: "completed_at", Value: created.Add(time.Minute)},
		},
		bson.D{
			{Key: "_id", Value: "deleting-uuid"}, {Key: "name", Value: "gone"}, {Key: "owner_uuid", Value: "owner-uuid"},
			{Key: "vm_uuid", Value: "vm-uuid"}, {Key: "vm_name", Value: "box"}, {Key: "kind", Value: "docker"},
			{Key: "image", Value: "docker:29-dind"}, {Key: "disk", Value: int64(20 << 30)}, {Key: "engine", Value: ""},
			{Key: "size", Value: int64(0)}, {Key: "state", Value: 4}, {Key: "reason", Value: ""},
			{Key: "created_at", Value: created}, {Key: "completed_at", Value: time.Time{}},
		},
	})
	require.NoError(t, err)

	require.NoError(t, snapshotsAsManifests.Up(ctx, database))
	require.NoError(t, snapshotsAsManifests.Up(ctx, database), "run again, it changes nothing")

	indexes, err := database.Collection("snapshots").Indexes().List(ctx)
	require.NoError(t, err)

	var names []string
	for indexes.Next(ctx) {
		names = append(names, indexes.Current.Lookup("name").StringValue())
	}

	assert.NotContains(t, names, "owner_uuid_1__id_-1")
	assert.NotContains(t, names, "vm_uuid_1")
	assert.Contains(t, names, "metadata.owner_uuid_1__id_-1")
	assert.Contains(t, names, "metadata.owners.uuid_1_metadata.owners.kind_1")

	// read where the migrations after this one leave them, workloads, as
	// `app migrate` goes on to apply them.
	migrateAfter(t, database, snapshotsAsManifests.Name)

	repository := resources.NewRepository(database)
	require.NoError(t, repository.EnsureKind(ctx, snapshotKind.Descriptor()), "the control plane indexes it as it is")

	ready, err := repository.GetOneByOwner(ctx, snapshotKind.Name, "owner-uuid", "ready-uuid")
	require.NoError(t, err)

	taken, err := kind.Decode[snapshotKind.Spec, snapshotKind.Status](ready.Raw)
	require.NoError(t, err)

	assert.Equal(t, snapshotKind.Name, taken.Kind)
	assert.Equal(t, "before the upgrade", taken.Metadata.Name)
	assert.Equal(t, []kind.Reference{{Kind: "vm", UUID: "vm-uuid"}}, taken.Metadata.Owners)
	assert.Equal(t, created, taken.Metadata.CreatedAt)
	assert.Equal(t, snapshotKind.VMRef{UUID: "vm-uuid", Name: "box"}, taken.Spec.VM)
	assert.Equal(t, snapshotKind.Ready, taken.Status.State)
	assert.Equal(t, snapshotKind.Ready, taken.Status.Expected)
	assert.Equal(t, vm.KindMachine, taken.Status.Flavor)
	assert.Equal(t, "ubuntu:24.04", taken.Status.Image)
	assert.Equal(t, uint64(10<<30), taken.Status.Disk)
	assert.Equal(t, "microsandbox/0.7.6", taken.Status.Engine)
	assert.Equal(t, int64(1<<30), taken.Status.Size)
	assert.Equal(t, created.Add(time.Minute), taken.Status.CompletedAt)
	assert.Equal(t, int64(1), ready.Version)

	deleting, err := repository.GetOne(ctx, snapshotKind.Name, "deleting-uuid")
	require.NoError(t, err)

	common, err := deleting.Common()
	require.NoError(t, err)
	assert.Equal(t, kind.Failed, common.State)
	assert.Equal(t, kind.Deleted, common.Expected, "its delete is asked for again")

	ofVM, total, err := repository.GetAll(ctx, snapshotKind.Name, resource.Filter{Parent: kind.Reference{Kind: "vm", UUID: "vm-uuid"}}, 0, 0)
	require.NoError(t, err)
	assert.Equal(t, uint(2), total, "both were taken of the vm")
	assert.Len(t, ofVM, 2)
}
