package stacks

import (
	"context"
	"errors"
	"time"

	"github.com/gofrs/uuid/v5"
	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"

	"github.com/khanzadimahdi/testproject/domain"
	"github.com/khanzadimahdi/testproject/domain/runner/stack"
)

const (
	collectionName = "stacks"
	queryTimeout   = 3 * time.Second
)

type StacksRepository struct {
	collection *mongo.Collection
}

var _ stack.Repository = &StacksRepository{}

func NewRepository(database *mongo.Database) *StacksRepository {
	if database == nil {
		panic("database should not be nil")
	}

	return &StacksRepository{
		collection: database.Collection(collectionName),
	}
}

func (r *StacksRepository) GetAll(ctx context.Context, offset uint, limit uint) ([]stack.Stack, error) {
	ctx, cancel := context.WithTimeout(ctx, queryTimeout)
	defer cancel()

	cur, err := r.collection.Find(
		ctx,
		bson.D{},
		options.Find().SetSkip(int64(offset)).SetLimit(int64(limit)).SetSort(bson.D{{Key: "_id", Value: -1}}),
	)
	if err != nil {
		return nil, err
	}
	defer cur.Close(ctx)

	items := make([]stack.Stack, 0, limit)
	for cur.Next(ctx) {
		var s StackBson

		if err := cur.Decode(&s); err != nil {
			return nil, err
		}

		items = append(items, toStack(&s))
	}

	return items, cur.Err()
}

// GetAllByOwner is GetAll of what one person asked for.
func (r *StacksRepository) GetAllByOwner(ctx context.Context, ownerUUID string, offset uint, limit uint) ([]stack.Stack, error) {
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

	items := make([]stack.Stack, 0, limit)
	for cur.Next(ctx) {
		var s StackBson

		if err := cur.Decode(&s); err != nil {
			return nil, err
		}

		items = append(items, toStack(&s))
	}

	if err := cur.Err(); err != nil {
		return nil, err
	}

	return items, nil
}

// CountByOwner is how many stacks one person has.
func (r *StacksRepository) CountByOwner(ctx context.Context, ownerUUID string) (uint, error) {
	ctx, cancel := context.WithTimeout(ctx, queryTimeout)
	defer cancel()

	count, err := r.collection.CountDocuments(ctx, bson.D{{Key: "owner_uuid", Value: ownerUUID}})
	if err != nil {
		return 0, err
	}

	return uint(count), nil
}

func (r *StacksRepository) GetOne(ctx context.Context, UUID string) (stack.Stack, error) {
	return r.findOne(ctx, bson.D{{Key: "_id", Value: UUID}})
}

func (r *StacksRepository) GetOneBySlug(ctx context.Context, slug string) (stack.Stack, error) {
	return r.findOne(ctx, bson.D{{Key: "slug", Value: slug}})
}

func (r *StacksRepository) findOne(ctx context.Context, filter bson.D) (stack.Stack, error) {
	ctx, cancel := context.WithTimeout(ctx, queryTimeout)
	defer cancel()

	var s StackBson
	if err := r.collection.FindOne(ctx, filter).Decode(&s); err != nil {
		if errors.Is(err, mongo.ErrNoDocuments) {
			err = domain.ErrNotExists
		}

		return stack.Stack{}, err
	}

	return toStack(&s), nil
}

func (r *StacksRepository) Save(ctx context.Context, s *stack.Stack) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, queryTimeout)
	defer cancel()

	if len(s.UUID) == 0 {
		UUID, err := uuid.NewV7()
		if err != nil {
			return "", err
		}
		s.UUID = UUID.String()
	}

	if s.CreatedAt.IsZero() {
		s.CreatedAt = time.Now()
	}

	if _, err := r.collection.UpdateOne(
		ctx,
		bson.D{{Key: "_id", Value: s.UUID}},
		bson.M{"$set": toBson(s)},
		options.UpdateOne().SetUpsert(true),
	); err != nil {
		return "", err
	}

	return s.UUID, nil
}

func (r *StacksRepository) Delete(ctx context.Context, UUID string) error {
	ctx, cancel := context.WithTimeout(ctx, queryTimeout)
	defer cancel()

	_, err := r.collection.DeleteOne(ctx, bson.D{{Key: "_id", Value: UUID}})

	return err
}

func (r *StacksRepository) Count(ctx context.Context) (uint, error) {
	ctx, cancel := context.WithTimeout(ctx, queryTimeout)
	defer cancel()

	c, err := r.collection.CountDocuments(ctx, bson.D{})

	return uint(c), err
}
