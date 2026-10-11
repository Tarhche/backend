//go:build integration

package leases

import (
	"context"
	"fmt"
	"os"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"

	"github.com/khanzadimahdi/testproject/domain/workload/graph"
	"github.com/khanzadimahdi/testproject/domain/workload/graph/leasestest"
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

	db := client.Database(fmt.Sprintf("leases_test_%s_%d", time.Now().Format("150405000000"), databases.Add(1)))
	t.Cleanup(func() {
		_ = db.Drop(context.Background())
		_ = client.Disconnect(context.Background())
	})

	return db
}

func TestRepository(t *testing.T) {
	t.Parallel()

	leasestest.Repository(t, func(t *testing.T) graph.Leases {
		return NewRepository(database(t))
	})
}

func TestRepository_stored(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	db := database(t)
	repository := NewRepository(db)

	// stored is the root's document as it is kept, but for its times, which
	// are the database's own, and whether each is there.
	stored := func(t *testing.T) (bson.M, map[string]bool) {
		t.Helper()

		var kept bson.M
		require.NoError(t, db.Collection("workload_leases").FindOne(ctx, bson.D{{Key: "_id", Value: "s-uuid"}}).Decode(&kept))

		times := make(map[string]bool)
		for _, field := range []string{"expires_at", "next_at", "updated_at"} {
			_, isTime := kept[field].(bson.DateTime)
			times[field] = isTime

			if isTime {
				kept[field] = "a time"
			}
		}

		return kept, times
	}

	_, err := repository.Take(ctx, "s-uuid", "one", time.Minute)
	require.NoError(t, err)

	t.Run("a lease taken is kept in workload_leases, under its root's uuid, by the names of its fields, nothing backing it off", func(t *testing.T) {
		kept, times := stored(t)

		assert.Equal(t, bson.M{
			"_id":        "s-uuid",
			"holder":     "one",
			"expires_at": "a time",
			"failures":   int32(0),
			"reason":     "",
			"next_at":    nil,
			"poked":      false,
			"updated_at": "a time",
		}, kept)
		assert.Equal(t, map[string]bool{"expires_at": true, "next_at": false, "updated_at": true}, times)

		collections, err := db.ListCollectionNames(ctx, bson.D{})
		require.NoError(t, err)
		assert.Equal(t, []string{"workload_leases"}, collections)
	})

	t.Run("one let go of after a failure is held by nobody, and backs its root off", func(t *testing.T) {
		_, err := repository.Release(ctx, "s-uuid", "one", graph.Ended{State: graph.StateFailed, Reason: "$failures is a reason, not a field", Backoff: graph.Backoff{First: 15 * time.Second, Most: 15 * time.Minute}})
		require.NoError(t, err)

		kept, times := stored(t)

		assert.Equal(t, bson.M{
			"_id":        "s-uuid",
			"holder":     nil,
			"expires_at": nil,
			"failures":   int32(1),
			"reason":     "$failures is a reason, not a field",
			"next_at":    "a time",
			"poked":      false,
			"updated_at": "a time",
		}, kept, "a reason is kept as it is said, whatever it starts with")
		assert.Equal(t, map[string]bool{"expires_at": false, "next_at": true, "updated_at": true}, times)
	})

	t.Run("one asked for while another holds it is kept poked, and taken, it is not", func(t *testing.T) {
		require.NoError(t, repository.Forget(ctx, "s-uuid"))

		_, err := repository.Take(ctx, "s-uuid", "two", time.Minute)
		require.NoError(t, err)

		_, err = repository.Take(ctx, "s-uuid", "three", time.Minute)
		require.ErrorIs(t, err, graph.ErrHeld)

		kept, _ := stored(t)
		assert.Equal(t, true, kept["poked"])
		assert.Equal(t, "two", kept["holder"])

		_, err = repository.Take(ctx, "s-uuid", "two", time.Minute)
		require.NoError(t, err)

		kept, _ = stored(t)
		assert.Equal(t, false, kept["poked"])
	})

	t.Run("indexed by nothing but its _id", func(t *testing.T) {
		cursor, err := db.Collection("workload_leases").Indexes().List(ctx)
		require.NoError(t, err)

		var indexes []struct {
			Name string `bson:"name"`
		}
		require.NoError(t, cursor.All(ctx, &indexes))

		require.Len(t, indexes, 1)
		assert.Equal(t, "_id_", indexes[0].Name)
	})
}

// TestRepository_taken holds a lease to being taken by one control plane of
// many that ask for it at once, whether it is kept already or not.
func TestRepository_taken(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	repository := NewRepository(database(t))

	// one lease no document is kept of, and one let go of before.
	_, err := repository.Take(ctx, "released-uuid", "before", time.Minute)
	require.NoError(t, err)

	_, err = repository.Release(ctx, "released-uuid", "before", graph.Ended{State: graph.StateDone})
	require.NoError(t, err)

	for _, root := range []string{"new-uuid", "released-uuid"} {
		t.Run(root, func(t *testing.T) {
			var (
				asking sync.WaitGroup
				lock   sync.Mutex
				took   []string
				held   int
			)

			for i := range 8 {
				holder := fmt.Sprintf("control-plane-%d", i)

				asking.Go(func() {
					_, err := repository.Take(ctx, root, holder, time.Minute)

					lock.Lock()
					defer lock.Unlock()

					switch {
					case err == nil:
						took = append(took, holder)
					case assert.ErrorIs(t, err, graph.ErrHeld):
						held++
					}
				})
			}

			asking.Wait()

			assert.Len(t, took, 1, "one takes it")
			assert.Equal(t, 7, held, "and every other finds it held")
		})
	}
}
