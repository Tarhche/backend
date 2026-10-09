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
	stackKind "github.com/khanzadimahdi/testproject/domain/workload/kinds/stack"
	"github.com/khanzadimahdi/testproject/domain/workload/resource"
	"github.com/khanzadimahdi/testproject/infrastructure/repository/mongodb/workload/resources"
)

// TestStacksAsManifests converts stacks kept as they were before a stack was a
// kind, on the server MONGO_TEST_URI names, and reads them back as the stack
// kind's repository does.
func TestStacksAsManifests(t *testing.T) {
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

	// what the stacks were kept as, and indexed by, before.
	require.NoError(t, indexWorkloadVMs.Up(ctx, database))

	created := time.Date(2026, 10, 5, 12, 0, 0, 0, time.UTC)

	_, err = database.Collection("vms").InsertOne(ctx, bson.D{{Key: "_id", Value: "vm-uuid"}, {Key: "slug", Value: "docker-x"}, {Key: "node_name", Value: "workload-orchestrator-01"}})
	require.NoError(t, err)

	_, err = database.Collection("stacks").InsertMany(ctx, []any{
		bson.D{
			{Key: "_id", Value: "running-uuid"}, {Key: "name", Value: "shop"}, {Key: "owner_uuid", Value: "owner-uuid"},
			{Key: "vm_uuid", Value: "vm-uuid"}, {Key: "slug", Value: "shop-abcde"}, {Key: "compose", Value: "services: {web: {image: nginx}}"},
			{Key: "expected_state", Value: 2}, {Key: "state", Value: 2}, {Key: "reason", Value: ""}, {Key: "output", Value: "Started"},
			{Key: "created_at", Value: created}, {Key: "updated_at", Value: created.Add(time.Hour)},
		},
		bson.D{
			{Key: "_id", Value: "removing-uuid"}, {Key: "name", Value: "old"}, {Key: "owner_uuid", Value: "owner-uuid"},
			{Key: "vm_uuid", Value: "vm-uuid"}, {Key: "slug", Value: "old-fghij"}, {Key: "compose", Value: "services: {db: {image: postgres}}"},
			{Key: "expected_state", Value: 2}, {Key: "state", Value: 7}, {Key: "reason", Value: ""}, {Key: "output", Value: ""},
			{Key: "created_at", Value: created}, {Key: "updated_at", Value: created},
		},
	})
	require.NoError(t, err)

	require.NoError(t, stacksAsManifests.Up(ctx, database))
	require.NoError(t, stacksAsManifests.Up(ctx, database), "run again, it changes nothing")

	indexes, err := database.Collection("stacks").Indexes().List(ctx)
	require.NoError(t, err)

	var names []string
	for indexes.Next(ctx) {
		names = append(names, indexes.Current.Lookup("name").StringValue())
	}

	assert.NotContains(t, names, "slug_1")
	assert.NotContains(t, names, "vm_uuid_1")
	assert.Contains(t, names, "metadata.slug_1")

	// read where the migrations after this one leave them, workloads, as
	// `app migrate` goes on to apply them.
	migrateAfter(t, database, stacksAsManifests.Name)

	repository := resources.NewRepository(database)
	require.NoError(t, repository.EnsureKind(ctx, stackKind.Descriptor()), "the control plane indexes it as it is")

	running, err := repository.GetOne(ctx, stackKind.Name, "running-uuid")
	require.NoError(t, err)

	shop, err := kind.Decode[stackKind.Spec, stackKind.Status](running.Raw)
	require.NoError(t, err)

	assert.Equal(t, "shop", shop.Metadata.Name)
	assert.Equal(t, "shop-abcde", shop.Metadata.Slug)
	assert.Equal(t, "owner-uuid", shop.Metadata.OwnerUUID)
	assert.Equal(t, []kind.Reference{{Kind: "vm", UUID: "vm-uuid"}}, shop.Metadata.Owners)
	assert.Equal(t, "workload-orchestrator-01", shop.Metadata.Node, "where its vm is")
	assert.Equal(t, created, shop.Metadata.CreatedAt)
	assert.Equal(t, stackKind.VMChoice{UUID: "vm-uuid"}, shop.Spec.VM)
	assert.Equal(t, "services: {web: {image: nginx}}", shop.Spec.Compose)
	assert.Equal(t, stackKind.Running, shop.Status.State)
	assert.Equal(t, stackKind.Running, shop.Status.Expected)
	assert.Equal(t, created.Add(time.Hour), shop.Status.Since)
	assert.Equal(t, "Started", shop.Status.Output)
	assert.Equal(t, int64(1), running.Version)

	removing, err := repository.GetOne(ctx, stackKind.Name, "removing-uuid")
	require.NoError(t, err)

	common, err := removing.Common()
	require.NoError(t, err)
	assert.Equal(t, kind.Failed, common.State)
	assert.Equal(t, kind.Deleted, common.Expected, "its delete is asked for again")

	held, total, err := repository.GetAll(ctx, stackKind.Name, resource.Filter{Node: "workload-orchestrator-01"}, 0, 0)
	require.NoError(t, err)
	assert.Equal(t, uint(2), total)
	assert.Len(t, held, 2)

	// a stack admitted after it, with a slug of its own, is kept beside them.
	_, err = repository.Create(ctx, resource.Record{Raw: kind.Raw{Kind: stackKind.Name, Metadata: kind.Metadata{Slug: "new-klmno", OwnerUUID: "owner-uuid"}}})
	require.NoError(t, err)

	_, err = repository.Create(ctx, resource.Record{Raw: kind.Raw{Kind: stackKind.Name, Metadata: kind.Metadata{Slug: "another-pqrst", OwnerUUID: "owner-uuid"}}})
	require.NoError(t, err)
}
