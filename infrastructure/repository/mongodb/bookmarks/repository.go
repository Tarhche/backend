package articles

import (
	"context"
	"errors"
	"time"

	"github.com/gofrs/uuid/v5"
	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"

	"github.com/khanzadimahdi/testproject/domain"
	"github.com/khanzadimahdi/testproject/domain/bookmark"
)

const (
	collectionName = "bookmarks"
	queryTimeout   = 3 * time.Second
)

type BookmarksRepository struct {
	collection *mongo.Collection
}

var _ bookmark.Repository = &BookmarksRepository{}

func NewRepository(database *mongo.Database) *BookmarksRepository {
	if database == nil {
		panic("database should not be nil")
	}

	return &BookmarksRepository{
		collection: database.Collection(collectionName),
	}
}

func (r *BookmarksRepository) Save(b *bookmark.Bookmark) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), queryTimeout)
	defer cancel()

	if len(b.UUID) == 0 {
		UUID, err := uuid.NewV7()
		if err != nil {
			return "", err
		}
		b.UUID = UUID.String()
		b.CreatedAt = time.Now()
	}

	update := BookmarkBson{
		UUID:       b.UUID,
		Title:      b.Title,
		ObjectUUID: b.ObjectUUID,
		ObjectType: b.ObjectType,
		OwnerUUID:  b.OwnerUUID,
		CreatedAt:  b.CreatedAt,
	}

	if _, err := r.collection.UpdateOne(
		ctx,
		bson.D{{Key: "_id", Value: b.UUID}},
		bson.M{"$set": update},
		options.UpdateOne().SetUpsert(true),
	); err != nil {
		return "", err
	}

	return b.ObjectUUID, nil
}

func (r *BookmarksRepository) Count(objectType string, objectUUID string) (uint, error) {
	ctx, cancel := context.WithTimeout(context.Background(), queryTimeout)
	defer cancel()

	filter := bson.M{
		"object_uuid": objectUUID,
		"object_type": objectType,
	}

	c, err := r.collection.CountDocuments(ctx, filter)
	if err != nil {
		return uint(c), err
	}

	return uint(c), nil
}

func (r *BookmarksRepository) GetAllByOwnerUUID(ownerUUID string, offset uint, limit uint) ([]bookmark.Bookmark, error) {
	ctx, cancel := context.WithTimeout(context.Background(), queryTimeout)
	defer cancel()

	o := int64(offset)
	l := int64(limit)
	desc := bson.D{{Key: "object_uuid", Value: -1}}

	filter := bson.M{
		"owner_uuid": ownerUUID,
	}

	cur, err := r.collection.Find(ctx, filter, options.Find().SetSkip(o).SetLimit(l).SetSort(desc))
	if err != nil {
		return nil, err
	}
	defer cur.Close(ctx)

	items := make([]bookmark.Bookmark, 0, limit)
	for cur.Next(ctx) {
		var b BookmarkBson

		if err := cur.Decode(&b); err != nil {
			return nil, err
		}
		items = append(items, bookmark.Bookmark{
			UUID:       b.ObjectUUID,
			Title:      b.Title,
			ObjectUUID: b.ObjectUUID,
			ObjectType: b.ObjectType,
			OwnerUUID:  b.OwnerUUID,
			CreatedAt:  b.CreatedAt,
		})
	}

	if err := cur.Err(); err != nil {
		return nil, err
	}

	return items, nil
}

func (r *BookmarksRepository) CountByOwnerUUID(ownerUUID string) (uint, error) {
	ctx, cancel := context.WithTimeout(context.Background(), queryTimeout)
	defer cancel()

	filter := bson.M{
		"owner_uuid": ownerUUID,
	}

	c, err := r.collection.CountDocuments(ctx, filter)
	if err != nil {
		return uint(c), err
	}

	return uint(c), nil
}

func (r *BookmarksRepository) GetByOwnerUUID(ownerUUID string, objectType string, objectUUID string) (bookmark.Bookmark, error) {
	ctx, cancel := context.WithTimeout(context.Background(), queryTimeout)
	defer cancel()

	filter := bson.M{
		"object_uuid": objectUUID,
		"object_type": objectType,
		"owner_uuid":  ownerUUID,
	}

	var b BookmarkBson
	if err := r.collection.FindOne(ctx, filter).Decode(&b); err != nil {
		if errors.Is(err, mongo.ErrNoDocuments) {
			err = domain.ErrNotExists
		}
		return bookmark.Bookmark{}, err
	}

	return bookmark.Bookmark{
		UUID:       b.ObjectUUID,
		Title:      b.Title,
		ObjectUUID: b.ObjectUUID,
		ObjectType: b.ObjectType,
		OwnerUUID:  b.OwnerUUID,
		CreatedAt:  b.CreatedAt,
	}, nil
}

func (r *BookmarksRepository) DeleteByOwnerUUID(ownerUUID string, objectType string, objectUUID string) error {
	ctx, cancel := context.WithTimeout(context.Background(), queryTimeout)
	defer cancel()

	filter := bson.M{
		"object_uuid": objectUUID,
		"object_type": objectType,
		"owner_uuid":  ownerUUID,
	}

	_, err := r.collection.DeleteOne(ctx, filter)

	return err
}
