package stacks

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

	"github.com/khanzadimahdi/testproject/domain/workload/runtime"
	"github.com/khanzadimahdi/testproject/domain/workload/stack"
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

func TestStacksRepository_Runtime(t *testing.T) {
	t.Parallel()

	t.Run("a stack's class is stored and read back", func(t *testing.T) {
		t.Parallel()

		repository := NewRepository(testDatabase(t))

		uuid, err := repository.Save(context.Background(), &stack.Stack{Name: "myapp", Slug: "myapp-abcde", Runtime: runtime.Firecracker})
		require.NoError(t, err)

		stored, err := repository.GetOne(context.Background(), uuid)
		require.NoError(t, err)
		assert.Equal(t, runtime.Firecracker, stored.Runtime)

		listed, err := repository.GetAll(context.Background(), 0, 10)
		require.NoError(t, err)
		require.Len(t, listed, 1)
		assert.Equal(t, runtime.Firecracker, listed[0].Runtime)
	})

	t.Run("a stack stored before there were classes is sysbox's", func(t *testing.T) {
		t.Parallel()

		database := testDatabase(t)

		_, err := database.Collection(collectionName).InsertOne(context.Background(), bson.D{
			{Key: "_id", Value: "legacy-uuid"},
			{Key: "name", Value: "myapp"},
			{Key: "slug", Value: "myapp-abcde"},
			{Key: "owner_uuid", Value: "owner-uuid"},
		})
		require.NoError(t, err)

		legacy, err := NewRepository(database).GetOne(context.Background(), "legacy-uuid")
		require.NoError(t, err)

		assert.Equal(t, runtime.Sysbox, legacy.Runtime)
	})
}
