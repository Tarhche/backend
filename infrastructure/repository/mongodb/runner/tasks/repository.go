package tasks

import (
	"context"
	"errors"
	"time"

	"github.com/gofrs/uuid/v5"
	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"

	"github.com/khanzadimahdi/testproject/domain"
	"github.com/khanzadimahdi/testproject/domain/runner/task"
)

const (
	collectionName = "tasks"
	queryTimeout   = 3 * time.Second
)

type TasksRepository struct {
	collection *mongo.Collection
}

var _ task.Repository = &TasksRepository{}

func NewRepository(database *mongo.Database) *TasksRepository {
	if database == nil {
		panic("database should not be nil")
	}

	return &TasksRepository{
		collection: database.Collection(collectionName),
	}
}

func (r *TasksRepository) GetAll(ctx context.Context, offset uint, limit uint) ([]task.Task, error) {
	ctx, cancel := context.WithTimeout(ctx, queryTimeout)
	defer cancel()

	o := int64(offset)
	l := int64(limit)
	desc := bson.D{{Key: "_id", Value: -1}}

	cur, err := r.collection.Find(ctx, bson.D{}, options.Find().SetSkip(o).SetLimit(l).SetSort(desc))
	if err != nil {
		return nil, err
	}
	defer cur.Close(ctx)

	items := make([]task.Task, 0, limit)
	for cur.Next(ctx) {
		var t TaskBson

		if err := cur.Decode(&t); err != nil {
			return nil, err
		}

		items = append(items, toTask(&t))
	}

	if err := cur.Err(); err != nil {
		return nil, err
	}

	return items, nil
}

// GetAllByStack returns the services of one stack, so a stack can be stopped,
// restarted or deleted as the single thing it is.
// GetAllByOwner is GetAll of what one person asked for.
func (r *TasksRepository) GetAllByOwner(ctx context.Context, ownerUUID string, offset uint, limit uint) ([]task.Task, error) {
	ctx, cancel := context.WithTimeout(ctx, queryTimeout)
	defer cancel()

	cur, err := r.collection.Find(
		ctx,
		bson.D{{Key: "owner_uuid", Value: ownerUUID}},
		options.Find().SetSkip(int64(offset)).SetLimit(int64(limit)).SetSort(bson.D{{Key: "_id", Value: -1}}),
	)
	if err != nil {
		return nil, err
	}
	defer cur.Close(ctx)

	items := make([]task.Task, 0, limit)
	for cur.Next(ctx) {
		var t TaskBson

		if err := cur.Decode(&t); err != nil {
			return nil, err
		}

		items = append(items, toTask(&t))
	}

	if err := cur.Err(); err != nil {
		return nil, err
	}

	return items, nil
}

// CountByOwner is how many containers one person has.
func (r *TasksRepository) CountByOwner(ctx context.Context, ownerUUID string) (uint, error) {
	ctx, cancel := context.WithTimeout(ctx, queryTimeout)
	defer cancel()

	count, err := r.collection.CountDocuments(ctx, bson.D{{Key: "owner_uuid", Value: ownerUUID}})
	if err != nil {
		return 0, err
	}

	return uint(count), nil
}

func (r *TasksRepository) GetAllByStack(ctx context.Context, stackUUID string) ([]task.Task, error) {
	ctx, cancel := context.WithTimeout(ctx, queryTimeout)
	defer cancel()

	cur, err := r.collection.Find(
		ctx,
		bson.D{{Key: "stack_uuid", Value: stackUUID}},
		options.Find().SetSort(bson.D{{Key: "service_name", Value: 1}}),
	)
	if err != nil {
		return nil, err
	}
	defer cur.Close(ctx)

	items := make([]task.Task, 0, 8)
	for cur.Next(ctx) {
		var t TaskBson

		if err := cur.Decode(&t); err != nil {
			return nil, err
		}

		items = append(items, toTask(&t))
	}

	return items, cur.Err()
}

func (r *TasksRepository) GetOne(ctx context.Context, UUID string) (task.Task, error) {
	return r.findOne(ctx, bson.D{{Key: "_id", Value: UUID}})
}

func (r *TasksRepository) GetOneByOwner(ctx context.Context, ownerUUID string, UUID string) (task.Task, error) {
	return r.findOne(ctx, bson.D{{Key: "_id", Value: UUID}, {Key: "owner_uuid", Value: ownerUUID}})
}

// GetOneBySlug finds a task by the unique name its ports are served on, which
// is how the ingress turns a hostname into a container.
func (r *TasksRepository) GetOneBySlug(ctx context.Context, slug string) (task.Task, error) {
	return r.findOne(ctx, bson.D{{Key: "slug", Value: slug}})
}

func (r *TasksRepository) findOne(ctx context.Context, filter bson.D) (task.Task, error) {
	ctx, cancel := context.WithTimeout(ctx, queryTimeout)
	defer cancel()

	var t TaskBson
	if err := r.collection.FindOne(ctx, filter).Decode(&t); err != nil {
		if errors.Is(err, mongo.ErrNoDocuments) {
			err = domain.ErrNotExists
		}

		return task.Task{}, err
	}

	return toTask(&t), nil
}

func (r *TasksRepository) Save(ctx context.Context, t *task.Task) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, queryTimeout)
	defer cancel()

	if len(t.UUID) == 0 {
		UUID, err := uuid.NewV7()
		if err != nil {
			return "", err
		}
		t.UUID = UUID.String()
	}

	// a task is saved on every state change, so its creation time is whatever
	// it already had rather than the time of the latest write.
	if t.CreatedAt.IsZero() {
		t.CreatedAt = time.Now()
	}

	update := toBson(t)

	if _, err := r.collection.UpdateOne(
		ctx,
		bson.D{{Key: "_id", Value: t.UUID}},
		bson.M{"$set": update},
		options.UpdateOne().SetUpsert(true),
	); err != nil {
		return "", err
	}

	return t.UUID, nil
}

func (r *TasksRepository) Delete(ctx context.Context, UUID string) error {
	ctx, cancel := context.WithTimeout(ctx, queryTimeout)
	defer cancel()

	_, err := r.collection.DeleteOne(ctx, bson.D{{Key: "_id", Value: UUID}})

	return err
}

func (r *TasksRepository) Count(ctx context.Context) (uint, error) {
	ctx, cancel := context.WithTimeout(ctx, queryTimeout)
	defer cancel()

	c, err := r.collection.CountDocuments(ctx, bson.D{})
	if err != nil {
		return uint(c), err
	}

	return uint(c), nil
}
