//go:build integration

package resources

import (
	"context"
	"fmt"
	"os"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"

	"github.com/khanzadimahdi/testproject/domain/workload/kind"
	"github.com/khanzadimahdi/testproject/domain/workload/resource"
	"github.com/khanzadimahdi/testproject/domain/workload/resource/resourcetest"
)

// databases tells apart the databases one run makes.
var databases atomic.Int64

// database is a fresh database on the server MONGO_TEST_URI names, or a
// skipped test when none is named.
func database(t *testing.T) *mongo.Database {
	t.Helper()

	uri := os.Getenv("MONGO_TEST_URI")
	if len(uri) == 0 {
		t.Skip("MONGO_TEST_URI names no database to test against")
	}

	client, err := mongo.Connect(options.Client().ApplyURI(uri))
	require.NoError(t, err)

	db := client.Database(fmt.Sprintf("resources_test_%s_%d", time.Now().Format("150405000000"), databases.Add(1)))
	t.Cleanup(func() {
		_ = db.Drop(context.Background())
		_ = client.Disconnect(context.Background())
	})

	return db
}

func TestRepository(t *testing.T) {
	t.Parallel()

	resourcetest.Repository(t, func(t *testing.T, kinds ...string) resource.Repository {
		repository := NewRepository(database(t))

		for _, name := range kinds {
			require.NoError(t, repository.EnsureKind(context.Background(), kind.Descriptor{Name: name, Plural: name + "s"}))
		}

		return repository
	})
}

func TestRepository_EnsureKind(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	db := database(t)
	repository := NewRepository(db)

	t.Run("a kind it was not told of is not kept", func(t *testing.T) {
		_, err := repository.GetOne(ctx, "fan", "fan-uuid")

		assert.ErrorIs(t, err, kind.ErrUnknownKind)
	})

	t.Run("a kind's resources are kept in a collection named by its plural, indexed the ways they are read", func(t *testing.T) {
		fan := kind.Descriptor{Name: "fan", Plural: "fans"}

		require.NoError(t, repository.EnsureKind(ctx, fan))
		require.NoError(t, repository.EnsureKind(ctx, fan), "it is told of a kind again at every start")

		_, err := repository.Create(ctx, resource.Record{Raw: kind.Raw{Kind: "fan", Metadata: kind.Metadata{UUID: "fan-uuid"}}})
		require.NoError(t, err)

		stored, err := db.Collection("fans").CountDocuments(ctx, bson.D{{Key: "_id", Value: "fan-uuid"}})
		require.NoError(t, err)
		assert.Equal(t, int64(1), stored)

		cursor, err := db.Collection("fans").Indexes().List(ctx)
		require.NoError(t, err)

		var indexes []struct {
			Key    bson.D `bson:"key"`
			Unique bool   `bson:"unique"`
			Sparse bool   `bson:"sparse"`
		}
		require.NoError(t, cursor.All(ctx, &indexes))

		keys := make(map[string]bool)
		for _, index := range indexes {
			var names string
			for _, key := range index.Key {
				names += key.Key + " "
			}

			keys[names] = true

			if names == "metadata.slug " {
				assert.True(t, index.Unique && index.Sparse, "a slug is unique among the resources that have one")
			}
		}

		for _, want := range []string{"_id ", "metadata.slug ", "metadata.owner_uuid _id ", "metadata.node ", "metadata.owners.uuid metadata.owners.kind "} {
			assert.True(t, keys[want], "an index on %q", want)
		}
	})

	t.Run("and one without a plural cannot be", func(t *testing.T) {
		assert.Error(t, repository.EnsureKind(ctx, kind.Descriptor{Name: "fan"}))
	})
}
