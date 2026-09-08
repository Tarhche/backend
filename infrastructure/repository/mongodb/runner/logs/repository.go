package logs

import (
	"context"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"

	"github.com/khanzadimahdi/testproject/domain/runner/container"
)

const (
	collectionName = "runner_logs"
	queryTimeout   = 5 * time.Second

	// defaultLimit is how many lines a read returns when the caller names no
	// limit of its own.
	defaultLimit = 500
)

// LogsRepository keeps the lines containers write, for as long as the task that
// produced them exists.
type LogsRepository struct {
	collection *mongo.Collection
}

var _ container.LogRepository = &LogsRepository{}

func NewRepository(database *mongo.Database) *LogsRepository {
	if database == nil {
		panic("database should not be nil")
	}

	return &LogsRepository{
		collection: database.Collection(collectionName),
	}
}

// EnsureIndexes creates the index a container's log is read by. Reading always
// asks for one task's lines in the order they were written, so that is the one
// index the collection needs.
func (r *LogsRepository) EnsureIndexes(ctx context.Context) error {
	ctx, cancel := context.WithTimeout(ctx, queryTimeout)
	defer cancel()

	_, err := r.collection.Indexes().CreateOne(ctx, mongo.IndexModel{
		Keys: bson.D{{Key: "task_uuid", Value: 1}, {Key: "at", Value: 1}},
	})

	return err
}

// Append stores lines, skipping the ones already held. A line is identified by
// its own content, so a worker replaying part of a stream it has already
// shipped costs a no-op write rather than a duplicate.
func (r *LogsRepository) Append(ctx context.Context, logs []container.Log) error {
	if len(logs) == 0 {
		return nil
	}

	ctx, cancel := context.WithTimeout(ctx, queryTimeout)
	defer cancel()

	writes := make([]mongo.WriteModel, len(logs))
	for i := range logs {
		stored := toBson(&logs[i])

		writes[i] = mongo.NewUpdateOneModel().
			SetFilter(bson.D{{Key: "_id", Value: stored.ID}}).
			SetUpdate(bson.M{"$setOnInsert": stored}).
			SetUpsert(true)
	}

	// unordered, so one line that cannot be written does not hold up the rest
	// of the batch.
	_, err := r.collection.BulkWrite(ctx, writes, options.BulkWrite().SetOrdered(false))

	return err
}

// Get returns a task's lines written after the given moment, oldest first, so a
// reader can page forward through a container's whole history.
func (r *LogsRepository) Get(ctx context.Context, taskUUID string, after time.Time, limit uint) ([]container.Log, error) {
	ctx, cancel := context.WithTimeout(ctx, queryTimeout)
	defer cancel()

	if limit == 0 {
		limit = defaultLimit
	}

	filter := bson.D{{Key: "task_uuid", Value: taskUUID}}
	if !after.IsZero() {
		filter = append(filter, bson.E{Key: "at", Value: bson.D{{Key: "$gt", Value: after}}})
	}

	cur, err := r.collection.Find(
		ctx,
		filter,
		options.Find().SetSort(bson.D{{Key: "at", Value: 1}}).SetLimit(int64(limit)),
	)
	if err != nil {
		return nil, err
	}
	defer cur.Close(ctx)

	items := make([]container.Log, 0, limit)
	for cur.Next(ctx) {
		var l LogBson

		if err := cur.Decode(&l); err != nil {
			return nil, err
		}

		items = append(items, toLog(&l))
	}

	return items, cur.Err()
}

// DeleteByTask drops everything a task ever wrote. Deleting the container is
// what ends its log, so this is called when the task itself goes.
func (r *LogsRepository) DeleteByTask(ctx context.Context, taskUUID string) error {
	ctx, cancel := context.WithTimeout(ctx, queryTimeout)
	defer cancel()

	_, err := r.collection.DeleteMany(ctx, bson.D{{Key: "task_uuid", Value: taskUUID}})

	return err
}

// Size reports how many bytes of content a task has stored, which is what the
// manager caps a chatty container against.
func (r *LogsRepository) Size(ctx context.Context, taskUUID string) (int64, error) {
	ctx, cancel := context.WithTimeout(ctx, queryTimeout)
	defer cancel()

	cur, err := r.collection.Aggregate(ctx, mongo.Pipeline{
		bson.D{{Key: "$match", Value: bson.D{{Key: "task_uuid", Value: taskUUID}}}},
		bson.D{{Key: "$group", Value: bson.D{
			{Key: "_id", Value: nil},
			{Key: "bytes", Value: bson.D{{Key: "$sum", Value: bson.D{{Key: "$strLenBytes", Value: "$content"}}}}},
		}}},
	})
	if err != nil {
		return 0, err
	}
	defer cur.Close(ctx)

	var result struct {
		Bytes int64 `bson:"bytes"`
	}

	if cur.Next(ctx) {
		if err := cur.Decode(&result); err != nil {
			return 0, err
		}
	}

	return result.Bytes, cur.Err()
}
