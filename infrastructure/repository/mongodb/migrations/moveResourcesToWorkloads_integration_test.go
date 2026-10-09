//go:build integration

package migrations

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"

	"github.com/khanzadimahdi/testproject/domain"
	"github.com/khanzadimahdi/testproject/domain/workload/kind"
	containerKind "github.com/khanzadimahdi/testproject/domain/workload/kinds/container"
	imageKind "github.com/khanzadimahdi/testproject/domain/workload/kinds/image"
	networkKind "github.com/khanzadimahdi/testproject/domain/workload/kinds/network"
	snapshotKind "github.com/khanzadimahdi/testproject/domain/workload/kinds/snapshot"
	stackKind "github.com/khanzadimahdi/testproject/domain/workload/kinds/stack"
	taskKind "github.com/khanzadimahdi/testproject/domain/workload/kinds/task"
	vmKind "github.com/khanzadimahdi/testproject/domain/workload/kinds/vm"
	volumeKind "github.com/khanzadimahdi/testproject/domain/workload/kinds/volume"
	"github.com/khanzadimahdi/testproject/domain/workload/resource"
	"github.com/khanzadimahdi/testproject/infrastructure/repository/mongodb/workload/resources"
)

// TestMoveResourcesToWorkloads moves what every kind kept in a collection of
// its own into workloads, on the server MONGO_TEST_URI names, and reads it
// back as the repository does.
func TestMoveResourcesToWorkloads(t *testing.T) {
	created := time.Date(2026, 10, 9, 12, 0, 0, 0, time.UTC)

	// a resource as the repository kept it in its kind's collection: of a
	// kind, under a uuid and a slug, at a version, and in a VM when it names
	// one.
	kept := func(kindName string, uuid string, slug string, version int64, vm string) bson.D {
		metadata := bson.D{{Key: "name", Value: kindName}}

		if len(slug) > 0 {
			metadata = append(metadata, bson.E{Key: "slug", Value: slug})
		}

		metadata = append(metadata, bson.E{Key: "owner_uuid", Value: "owner-uuid"})

		if len(vm) > 0 {
			metadata = append(metadata, bson.E{Key: "owners", Value: bson.A{bson.D{{Key: "kind", Value: "vm"}, {Key: "uuid", Value: vm}}}})
		}

		metadata = append(metadata,
			bson.E{Key: "node", Value: "workload-orchestrator-01"},
			bson.E{Key: "created_at", Value: created},
			bson.E{Key: "updated_at", Value: created},
		)

		return bson.D{
			{Key: "_id", Value: uuid},
			{Key: "kind", Value: kindName},
			{Key: "metadata", Value: metadata},
			{Key: "spec", Value: bson.D{{Key: "image", Value: "ubuntu:24.04"}}},
			{Key: "status", Value: bson.D{{Key: "state", Value: "running"}, {Key: "expected", Value: "running"}}},
			{Key: "version", Value: version},
			{Key: "control", Value: bson.D{{Key: "attempts", Value: 2}}},
		}
	}

	// keep keeps in each kind's collection what it is given, indexed as the
	// repository indexed it.
	keep := func(t *testing.T, database *mongo.Database, collections map[string][]any) {
		t.Helper()

		for name, documents := range collections {
			collection := database.Collection(name)

			_, err := collection.Indexes().CreateMany(context.Background(), []mongo.IndexModel{
				{Keys: bson.D{{Key: "metadata.slug", Value: 1}}, Options: options.Index().SetUnique(true).SetSparse(true)},
				{Keys: bson.D{{Key: "metadata.owner_uuid", Value: 1}, {Key: "_id", Value: -1}}},
				{Keys: bson.D{{Key: "metadata.node", Value: 1}}},
				{Keys: bson.D{{Key: "metadata.owners.uuid", Value: 1}, {Key: "metadata.owners.kind", Value: 1}}},
			})
			require.NoError(t, err)

			_, err = collection.InsertMany(context.Background(), documents)
			require.NoError(t, err)
		}
	}

	t.Run("every kind's resources are moved as they are, and its collection dropped", func(t *testing.T) {
		ctx := context.Background()
		database := freshDatabase(t)

		collections := map[string][]any{
			"vms":        {kept("vm", "vm-uuid", "box-abcde", 7, ""), kept("vm", "docker-uuid", "docker-fghij", 3, "")},
			"snapshots":  {kept("snapshot", "snapshot-uuid", "", 2, "vm-uuid")},
			"stacks":     {kept("stack", "stack-uuid", "shop-klmno", 4, "docker-uuid")},
			"tasks":      {kept("task", "task-uuid", "request-pqrst", 1, "")},
			"containers": {kept("container", "container-uuid", "", 5, "docker-uuid")},
			"images":     {kept("image", "image-uuid", "", 1, "docker-uuid")},
			"networks":   {kept("network", "network-uuid", "", 1, "docker-uuid")},
			"volumes":    {kept("volume", "volume-uuid", "", 1, "docker-uuid")},
		}

		keep(t, database, collections)

		// what a run cut short had moved already, at a version of its own.
		_, err := database.Collection("workloads").InsertOne(ctx, kept("vm", "docker-uuid", "docker-fghij", 8, ""))
		require.NoError(t, err)

		require.NoError(t, moveResourcesToWorkloads.Up(ctx, database))
		require.NoError(t, moveResourcesToWorkloads.Up(ctx, database), "run again, it changes nothing")

		names, err := database.ListCollectionNames(ctx, bson.D{})
		require.NoError(t, err)

		for name := range collections {
			assert.NotContains(t, names, name, "a kind's collection is dropped once it is moved")
		}

		workloads := database.Collection("workloads")

		moved, err := workloads.CountDocuments(ctx, bson.D{})
		require.NoError(t, err)
		assert.Equal(t, int64(9), moved, "every resource is moved, once")

		for uuid, want := range map[string]struct {
			kind    string
			version int64
		}{
			"vm-uuid":        {kind: "vm", version: 7},
			"docker-uuid":    {kind: "vm", version: 8},
			"snapshot-uuid":  {kind: "snapshot", version: 2},
			"stack-uuid":     {kind: "stack", version: 4},
			"task-uuid":      {kind: "task", version: 1},
			"container-uuid": {kind: "container", version: 5},
			"image-uuid":     {kind: "image", version: 1},
			"network-uuid":   {kind: "network", version: 1},
			"volume-uuid":    {kind: "volume", version: 1},
		} {
			var stored struct {
				Kind    string `bson:"kind"`
				Version int64  `bson:"version"`
			}

			require.NoError(t, workloads.FindOne(ctx, bson.D{{Key: "_id", Value: uuid}}).Decode(&stored), uuid)
			assert.Equal(t, want.kind, stored.Kind, "%s keeps its kind", uuid)
			assert.Equal(t, want.version, stored.Version, "%s keeps its version, or what was moved of it before does", uuid)
		}

		repository := resources.NewRepository(database)

		for _, d := range []kind.Descriptor{
			vmKind.Descriptor(), snapshotKind.Descriptor(), stackKind.Descriptor(), taskKind.Descriptor(),
			containerKind.Descriptor(), imageKind.Descriptor(), networkKind.Descriptor(), volumeKind.Descriptor(),
		} {
			require.NoError(t, repository.EnsureKind(ctx, d), "the control plane indexes it as the move did")
		}

		vm, err := repository.GetOne(ctx, vmKind.Name, "vm-uuid")
		require.NoError(t, err)
		assert.Equal(t, int64(7), vm.Version)
		assert.Equal(t, "box-abcde", vm.Metadata.Slug)
		assert.Equal(t, created, vm.Metadata.CreatedAt)
		assert.Equal(t, 2, vm.Attempts, "and what the control plane was in the middle of asking it")

		stack, err := repository.GetOneBySlug(ctx, stackKind.Name, "shop-klmno")
		require.NoError(t, err)
		assert.Equal(t, "stack-uuid", stack.Metadata.UUID)

		inDocker, total, err := repository.GetAll(ctx, containerKind.Name, resource.Filter{Parent: kind.Reference{Kind: "vm", UUID: "docker-uuid"}}, 0, 0)
		require.NoError(t, err)
		assert.Equal(t, uint(1), total, "what lives in a vm is of its kind")
		require.Len(t, inDocker, 1)
		assert.Equal(t, "container-uuid", inDocker[0].Metadata.UUID)

		held, total, err := repository.GetAll(ctx, vmKind.Name, resource.Filter{Node: "workload-orchestrator-01"}, 0, 0)
		require.NoError(t, err)
		assert.Equal(t, uint(2), total, "and what a node holds")
		assert.Len(t, held, 2)

		_, err = repository.GetOne(ctx, stackKind.Name, "vm-uuid")
		assert.ErrorIs(t, err, domain.ErrNotExists, "a vm is not a stack")

		_, err = repository.Create(ctx, resource.Record{Raw: kind.Raw{Kind: taskKind.Name, Metadata: kind.Metadata{Slug: "shop-klmno", OwnerUUID: "guest"}}})
		assert.ErrorIs(t, err, domain.ErrAlreadyExists, "a task is not reached under a stack's slug")
	})

	t.Run("two kinds' resources under one slug are not moved, and nothing is dropped", func(t *testing.T) {
		ctx := context.Background()
		database := freshDatabase(t)

		keep(t, database, map[string][]any{
			"vms":    {kept("vm", "vm-uuid", "web-abcde", 1, "")},
			"stacks": {kept("stack", "stack-uuid", "web-abcde", 1, "")},
			"tasks":  {kept("task", "task-uuid", "request-fghij", 1, "")},
		})

		err := moveResourcesToWorkloads.Up(ctx, database)
		require.Error(t, err)
		assert.Contains(t, err.Error(), `the vm "vm-uuid" in vms and the stack "stack-uuid" in stacks share the slug "web-abcde"`)

		migrator := NewMigrator(database, moveResourcesToWorkloads)

		_, err = migrator.Migrate(ctx)
		assert.ErrorContains(t, err, moveResourcesToWorkloads.Name+": ")

		pending, err := migrator.Pending(ctx)
		require.NoError(t, err)
		assert.Equal(t, []string{moveResourcesToWorkloads.Name}, pending, "it is not recorded, so the control plane goes on waiting")

		for name, want := range map[string]int64{"vms": 1, "stacks": 1, "tasks": 1, "workloads": 0} {
			count, err := database.Collection(name).CountDocuments(ctx, bson.D{})
			require.NoError(t, err)
			assert.Equal(t, want, count, "%s keeps what it kept", name)
		}
	})
}
