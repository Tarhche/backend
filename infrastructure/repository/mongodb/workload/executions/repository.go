// Package executions keeps how each node of each root's plan went in
// MongoDB, in workload_executions: one document for a node of a root, as it
// went the last time the root's plan was carried out,
//
//	{_id: "<root uuid>|<node>", root: {kind, uuid}, generation, node, kind,
//	 action, level, state, reason, ref: {kind, uuid}, started_at,
//	 finished_at, updated_at}
//
// written over each time it is saved, with $set, upserted. Only one control
// plane carries a root's plan out at a time, so what was saved last is what
// is so, and no version is kept to hold two writers apart. Every field is set
// each time, a record or a time left empty as null, so that nothing of what
// was saved before is kept.
//
// A root's executions are read together, in the order of their levels and
// then of their nodes, and forgotten together, or those of generations
// before one, through an index on the root's uuid, the level and the node,
// which is made at start (EnsureIndexes), as the task logs' is.
package executions

import (
	"context"
	"fmt"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"

	"github.com/khanzadimahdi/testproject/domain/workload/graph"
)

const (
	collectionName = "workload_executions"
	queryTimeout   = 3 * time.Second
)

// Repository keeps how each node of each root's plan went.
type Repository struct {
	collection *mongo.Collection
}

var _ graph.Executions = &Repository{}

func NewRepository(database *mongo.Database) *Repository {
	if database == nil {
		panic("database should not be nil")
	}

	return &Repository{collection: database.Collection(collectionName)}
}

// EnsureIndexes makes sure the collection is indexed the way it is read: a
// root's executions, in the order of their levels and then of their nodes,
// which is how they are forgotten too. It may be indexed so already.
func (r *Repository) EnsureIndexes(ctx context.Context) error {
	ctx, cancel := context.WithTimeout(ctx, queryTimeout)
	defer cancel()

	if _, err := r.collection.Indexes().CreateOne(ctx, mongo.IndexModel{
		Keys: bson.D{{Key: "root.uuid", Value: 1}, {Key: "level", Value: 1}, {Key: "node", Value: 1}},
	}); err != nil {
		return fmt.Errorf("%s cannot be indexed: %w", collectionName, err)
	}

	return nil
}

func (r *Repository) Save(ctx context.Context, e graph.Execution) error {
	id, err := idOf(e)
	if err != nil {
		return err
	}

	ctx, cancel := context.WithTimeout(ctx, queryTimeout)
	defer cancel()

	_, err = r.collection.UpdateOne(
		ctx,
		bson.D{{Key: "_id", Value: id}},
		bson.D{{Key: "$set", Value: toDocument(e)}},
		options.UpdateOne().SetUpsert(true),
	)

	return err
}

func (r *Repository) GetByRoot(ctx context.Context, rootUUID string) ([]graph.Execution, error) {
	ctx, cancel := context.WithTimeout(ctx, queryTimeout)
	defer cancel()

	cursor, err := r.collection.Find(
		ctx,
		bson.D{{Key: "root.uuid", Value: rootUUID}},
		options.Find().SetSort(bson.D{{Key: "level", Value: 1}, {Key: "node", Value: 1}}),
	)
	if err != nil {
		return nil, err
	}
	defer cursor.Close(ctx)

	executions := make([]graph.Execution, 0)
	for cursor.Next(ctx) {
		var stored document
		if err := cursor.Decode(&stored); err != nil {
			return nil, err
		}

		executions = append(executions, toExecution(stored))
	}

	if err := cursor.Err(); err != nil {
		return nil, err
	}

	return executions, nil
}

func (r *Repository) DeleteByRoot(ctx context.Context, rootUUID string) error {
	ctx, cancel := context.WithTimeout(ctx, queryTimeout)
	defer cancel()

	_, err := r.collection.DeleteMany(ctx, bson.D{{Key: "root.uuid", Value: rootUUID}})

	return err
}

func (r *Repository) DeleteBefore(ctx context.Context, rootUUID string, generation int64) error {
	ctx, cancel := context.WithTimeout(ctx, queryTimeout)
	defer cancel()

	_, err := r.collection.DeleteMany(ctx, bson.D{{Key: "root.uuid", Value: rootUUID}, {Key: "generation", Value: bson.D{{Key: "$lt", Value: generation}}}})

	return err
}
