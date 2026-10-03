package tasks

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"

	"github.com/khanzadimahdi/testproject/domain/workload/network"
	"github.com/khanzadimahdi/testproject/domain/workload/runtime"
	"github.com/khanzadimahdi/testproject/domain/workload/task"
)

// testDatabase is a database of its own on the MongoDB that
// WORKLOAD_TEST_MONGO_URI names, dropped when the test ends. What MongoDB does
// with what the repository writes is something only a MongoDB can answer, so
// without one the test is skipped.
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

func TestTasksRepository_Runtime(t *testing.T) {
	t.Parallel()

	t.Run("a task's class is stored and read back", func(t *testing.T) {
		t.Parallel()

		database := testDatabase(t)
		repository := NewRepository(database)

		uuid, err := repository.Save(context.Background(), &task.Task{
			Name:          "web",
			Image:         "nginx:alpine",
			Runtime:       runtime.Firecracker,
			NetworkPolicy: network.PolicyIsolated,
		})
		require.NoError(t, err)

		stored, err := repository.GetOne(context.Background(), uuid)
		require.NoError(t, err)
		assert.Equal(t, runtime.Firecracker, stored.Runtime)

		var raw bson.M
		require.NoError(t, database.Collection(collectionName).FindOne(context.Background(), bson.D{{Key: "_id", Value: uuid}}).Decode(&raw))
		assert.Equal(t, "firecracker", raw["runtime"])
	})

	t.Run("a task stored before there were classes is sysbox's, and says so once it is saved again", func(t *testing.T) {
		t.Parallel()

		database := testDatabase(t)
		repository := NewRepository(database)

		_, err := database.Collection(collectionName).InsertOne(context.Background(), bson.D{
			{Key: "_id", Value: "legacy-uuid"},
			{Key: "name", Value: "web"},
			{Key: "image", Value: "nginx:alpine"},
			{Key: "current_state", Value: 3},
			{Key: "owner_uuid", Value: "owner-uuid"},
		})
		require.NoError(t, err)

		legacy, err := repository.GetOne(context.Background(), "legacy-uuid")
		require.NoError(t, err)
		assert.Equal(t, runtime.Sysbox, legacy.Runtime)

		_, err = repository.Save(context.Background(), &legacy)
		require.NoError(t, err)

		var raw bson.M
		require.NoError(t, database.Collection(collectionName).FindOne(context.Background(), bson.D{{Key: "_id", Value: "legacy-uuid"}}).Decode(&raw))
		assert.Equal(t, "sysbox", raw["runtime"])
	})
}
