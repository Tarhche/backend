//go:build integration

package executions

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

	"github.com/khanzadimahdi/testproject/domain/workload/graph"
	"github.com/khanzadimahdi/testproject/domain/workload/graph/executionstest"
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

	db := client.Database(fmt.Sprintf("executions_test_%s_%d", time.Now().Format("150405000000"), databases.Add(1)))
	t.Cleanup(func() {
		_ = db.Drop(context.Background())
		_ = client.Disconnect(context.Background())
	})

	return db
}

func TestRepository(t *testing.T) {
	t.Parallel()

	executionstest.Repository(t, func(t *testing.T) graph.Executions {
		repository := NewRepository(database(t))
		require.NoError(t, repository.EnsureIndexes(context.Background()))

		return repository
	})
}

func TestRepository_stored(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	db := database(t)
	repository := NewRepository(db)

	require.NoError(t, repository.EnsureIndexes(ctx))
	require.NoError(t, repository.EnsureIndexes(ctx), "it is indexed again at every start")

	at := time.Date(2026, 10, 10, 12, 0, 0, 0, time.UTC)

	require.NoError(t, repository.Save(ctx, graph.Execution{
		Root:       graph.Ref{Kind: "stack", UUID: "s-uuid"},
		Generation: 1,
		Node:       "network/shop_default",
		Kind:       "network",
		Action:     graph.ActionCreate,
		Level:      1,
		State:      graph.StateFailed,
		Reason:     "broken",
		StartedAt:  at,
		FinishedAt: at,
		UpdatedAt:  at,
	}))

	t.Run("an execution is kept in workload_executions, under its root's uuid and its node, by the names of its fields", func(t *testing.T) {
		// an upsert's $set writes the fields of a document it makes in the
		// order of their names, so they are held to their names alone.
		var stored bson.M
		require.NoError(t, db.Collection("workload_executions").FindOne(ctx, bson.D{{Key: "_id", Value: "s-uuid|network/shop_default"}}).Decode(&stored))

		assert.Equal(t, bson.M{
			"_id":         "s-uuid|network/shop_default",
			"root":        bson.D{{Key: "kind", Value: "stack"}, {Key: "uuid", Value: "s-uuid"}},
			"generation":  int64(1),
			"node":        "network/shop_default",
			"kind":        "network",
			"action":      "create",
			"level":       int32(1),
			"state":       "failed",
			"reason":      "broken",
			"ref":         nil,
			"started_at":  bson.NewDateTimeFromTime(at),
			"finished_at": bson.NewDateTimeFromTime(at),
			"updated_at":  bson.NewDateTimeFromTime(at),
		}, stored)

		collections, err := db.ListCollectionNames(ctx, bson.D{})
		require.NoError(t, err)
		assert.Equal(t, []string{"workload_executions"}, collections)
	})

	t.Run("indexed by a root's uuid, its level and its node, and nothing else", func(t *testing.T) {
		cursor, err := db.Collection("workload_executions").Indexes().List(ctx)
		require.NoError(t, err)

		var indexes []struct {
			Key bson.D `bson:"key"`
		}
		require.NoError(t, cursor.All(ctx, &indexes))

		var keys []string
		for _, index := range indexes {
			var names string
			for _, key := range index.Key {
				names += key.Key + " "
			}

			keys = append(keys, names)
		}

		assert.ElementsMatch(t, []string{"_id ", "root.uuid level node "}, keys)
	})
}
