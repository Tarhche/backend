package migrations

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"os"
	"slices"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
)

func TestBackfillRuntime(t *testing.T) {
	t.Parallel()

	t.Run("it comes after everything released before it", func(t *testing.T) {
		t.Parallel()

		all := All()

		names := make([]string, len(all))
		for i, migration := range all {
			names[i] = migration.Name
		}

		assert.Equal(t, backfillRuntime.Name, names[len(names)-1])
		assert.Less(t, slices.Index(names, renameRunnerToWorkload.Name), slices.Index(names, backfillRuntime.Name))

		slices.Sort(names)
		assert.Len(t, slices.Compact(names), len(all), "a name is a migration")
	})

	t.Run("it touches only what names no class", func(t *testing.T) {
		t.Parallel()

		// a missing field and a null both match null; an empty one is
		// what a class written as nothing looks like.
		assert.Equal(t, bson.D{{Key: "runtime", Value: bson.D{{Key: "$in", Value: bson.A{nil, ""}}}}}, withoutRuntime())
	})

	t.Run("what ran before there were classes is written down as sysbox, once", func(t *testing.T) {
		t.Parallel()

		ctx := context.Background()
		database := testDatabase(t)

		_, err := database.Collection("tasks").InsertMany(ctx, []any{
			bson.D{{Key: "_id", Value: "missing"}, {Key: "name", Value: "web"}},
			bson.D{{Key: "_id", Value: "empty"}, {Key: "name", Value: "api"}, {Key: "runtime", Value: ""}},
			bson.D{{Key: "_id", Value: "null"}, {Key: "name", Value: "db"}, {Key: "runtime", Value: nil}},
			bson.D{{Key: "_id", Value: "named"}, {Key: "name", Value: "vm"}, {Key: "runtime", Value: "firecracker"}},
		})
		require.NoError(t, err)

		_, err = database.Collection("stacks").InsertMany(ctx, []any{
			bson.D{{Key: "_id", Value: "legacy"}, {Key: "name", Value: "myapp"}},
			bson.D{{Key: "_id", Value: "named"}, {Key: "name", Value: "vms"}, {Key: "runtime", Value: "firecracker"}},
		})
		require.NoError(t, err)

		applied, err := NewMigrator(database, All()...).Migrate(ctx)
		require.NoError(t, err)
		assert.Contains(t, applied, backfillRuntime.Name)

		runtimes := func(collection string) map[string]any {
			cursor, err := database.Collection(collection).Find(ctx, bson.D{})
			require.NoError(t, err)

			var documents []bson.M
			require.NoError(t, cursor.All(ctx, &documents))

			found := make(map[string]any, len(documents))
			for _, document := range documents {
				found[document["_id"].(string)] = document["runtime"]
			}

			return found
		}

		want := map[string]map[string]any{
			"tasks":  {"missing": "sysbox", "empty": "sysbox", "null": "sysbox", "named": "firecracker"},
			"stacks": {"legacy": "sysbox", "named": "firecracker"},
		}

		for collection, expected := range want {
			assert.Equal(t, expected, runtimes(collection), collection)
		}

		// a migrator run again applies nothing it has applied,
		applied, err = NewMigrator(database, All()...).Migrate(ctx)
		require.NoError(t, err)
		assert.Empty(t, applied)

		// and the migration itself, run again, finds nothing left to do.
		require.NoError(t, backfillRuntime.Up(ctx, database))

		for collection, expected := range want {
			assert.Equal(t, expected, runtimes(collection), collection)
		}
	})
}

// testDatabase is a database of its own on the MongoDB that
// WORKLOAD_TEST_MONGO_URI names, dropped when the test ends. What a migration
// does is something only a MongoDB can answer, so without one the test is
// skipped.
func testDatabase(t *testing.T) *mongo.Database {
	t.Helper()

	uri := os.Getenv("WORKLOAD_TEST_MONGO_URI")
	if len(uri) == 0 {
		t.Skip("WORKLOAD_TEST_MONGO_URI names no MongoDB to test against")
	}

	client, err := mongo.Connect(options.Client().ApplyURI(uri))
	require.NoError(t, err)

	name := make([]byte, 6)
	_, err = rand.Read(name)
	require.NoError(t, err)

	database := client.Database("test_" + hex.EncodeToString(name))

	t.Cleanup(func() {
		ctx := context.Background()

		_ = database.Drop(ctx)
		_ = client.Disconnect(ctx)
	})

	return database
}
