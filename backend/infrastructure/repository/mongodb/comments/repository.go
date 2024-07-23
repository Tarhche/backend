package comments

import (
	"context"
	"errors"
	"time"

	"github.com/gofrs/uuid/v5"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"

	"github.com/khanzadimahdi/testproject/domain"
	"github.com/khanzadimahdi/testproject/domain/author"
	"github.com/khanzadimahdi/testproject/domain/comment"
)

const (
	collectionName = "comments"
	queryTimeout   = 3 * time.Second
)

type CommentsRepository struct {
	collection *mongo.Collection
}

var _ comment.Repository = &CommentsRepository{}

func NewRepository(database *mongo.Database) *CommentsRepository {
	if database == nil {
		panic("database should not be nil")
	}

	return &CommentsRepository{
		collection: database.Collection(collectionName),
	}
}

func (r *CommentsRepository) GetAll(offset uint, limit uint) ([]comment.Comment, error) {
	ctx, cancel := context.WithTimeout(context.Background(), queryTimeout)
	defer cancel()

	o := int64(offset)
	l := int64(limit)
	desc := bson.D{{Key: "_id", Value: -1}}

	cur, err := r.collection.Find(ctx, bson.D{}, &options.FindOptions{
		Skip:  &o,
		Limit: &l,
		Sort:  desc,
	})

	if err != nil {
		return nil, err
	}

	defer cur.Close(ctx)

	items := make([]comment.Comment, 0, limit)
	for cur.Next(ctx) {
		var c CommentBson

		if err := cur.Decode(&c); err != nil {
			return nil, err
		}
		items = append(items, comment.Comment{
			UUID: c.UUID,
			Body: c.Body,
			Author: author.Author{
				UUID: c.AuthorUUID,
			},
			ParentUUID: c.ParentUUID,
			ObjectUUID: c.ObjectUUID,
			ObjectType: c.ObjectType,
			ApprovedAt: c.ApprovedAt,
			CreatedAt:  c.CreatedAt,
		})
	}

	if err := cur.Err(); err != nil {
		return nil, err
	}

	return items, nil
}

func (r *CommentsRepository) GetOne(UUID string) (comment.Comment, error) {
	ctx, cancel := context.WithTimeout(context.Background(), queryTimeout)
	defer cancel()

	filter := bson.D{{Key: "_id", Value: UUID}}

	var c CommentBson
	if err := r.collection.FindOne(ctx, filter, nil).Decode(&c); err != nil {
		if errors.Is(err, mongo.ErrNoDocuments) {
			err = domain.ErrNotExists
		}
		return comment.Comment{}, err
	}

	return comment.Comment{
		UUID: c.UUID,
		Body: c.Body,
		Author: author.Author{
			UUID: c.AuthorUUID,
		},
		ParentUUID: c.ParentUUID,
		ObjectUUID: c.ObjectUUID,
		ObjectType: c.ObjectType,
		ApprovedAt: c.ApprovedAt,
		CreatedAt:  c.CreatedAt,
	}, nil
}

func (r *CommentsRepository) Count() (uint, error) {
	ctx, cancel := context.WithTimeout(context.Background(), queryTimeout)
	defer cancel()

	c, err := r.collection.CountDocuments(ctx, bson.D{}, nil)
	if err != nil {
		return uint(c), err
	}

	return uint(c), nil
}

func (r *CommentsRepository) Save(c *comment.Comment) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), queryTimeout)
	defer cancel()

	if len(c.UUID) == 0 {
		UUID, err := uuid.NewV7()
		if err != nil {
			return "", err
		}
		c.UUID = UUID.String()
		c.CreatedAt = time.Now()
	}

	update := CommentBson{
		UUID:       c.UUID,
		Body:       c.Body,
		AuthorUUID: c.Author.UUID,
		ParentUUID: c.ParentUUID,
		ObjectUUID: c.ObjectUUID,
		ObjectType: c.ObjectType,
		ApprovedAt: c.ApprovedAt,
		CreatedAt:  c.CreatedAt,
		UpdatedAt:  time.Now(),
	}

	upsert := true
	if _, err := r.collection.UpdateOne(
		ctx,
		bson.D{{Key: "_id", Value: c.UUID}},
		SetWrapper{Set: update},
		&options.UpdateOptions{Upsert: &upsert},
	); err != nil {
		return "", err
	}

	return c.UUID, nil
}

func (r *CommentsRepository) Delete(UUID string) error {
	ctx, cancel := context.WithTimeout(context.Background(), queryTimeout)
	defer cancel()

	_, err := r.collection.DeleteOne(ctx, bson.D{{Key: "_id", Value: UUID}}, nil)

	return err
}

func (r *CommentsRepository) GetApprovedByObjectUUID(objectType string, UUID string, offset uint, limit uint) ([]comment.Comment, error) {
	ctx, cancel := context.WithTimeout(context.Background(), queryTimeout)
	defer cancel()

	o := int64(offset)
	l := int64(limit)
	desc := bson.D{{Key: "approved_at", Value: -1}}

	filter := bson.D{
		{
			Key: "$and",
			Value: bson.A{
				bson.D{{Key: "object_uuid", Value: UUID}},
				bson.D{{Key: "object_type", Value: objectType}},
				bson.D{
					{
						Key: "approved_at",
						Value: bson.M{
							"$lte": primitive.NewDateTimeFromTime(time.Now()),
						},
					},
				},
			},
		},
	}

	cur, err := r.collection.Find(ctx, filter, &options.FindOptions{
		Skip:  &o,
		Limit: &l,
		Sort:  desc,
	})

	if err != nil {
		return nil, err
	}

	defer cur.Close(ctx)

	items := make([]comment.Comment, 0, limit)
	for cur.Next(ctx) {
		var c CommentBson

		if err := cur.Decode(&c); err != nil {
			return nil, err
		}
		items = append(items, comment.Comment{
			UUID: c.UUID,
			Body: c.Body,
			Author: author.Author{
				UUID: c.AuthorUUID,
			},
			ParentUUID: c.ParentUUID,
			ObjectUUID: c.ObjectUUID,
			ObjectType: c.ObjectType,
			ApprovedAt: c.ApprovedAt,
			CreatedAt:  c.CreatedAt,
		})
	}

	if err := cur.Err(); err != nil {
		return nil, err
	}

	return items, nil
}

func (r *CommentsRepository) CountApprovedByObjectUUID(objectType string, UUID string) (uint, error) {
	ctx, cancel := context.WithTimeout(context.Background(), queryTimeout)
	defer cancel()

	filter := bson.D{
		{
			Key: "$and",
			Value: bson.A{
				bson.D{{Key: "object_uuid", Value: UUID}},
				bson.D{{Key: "object_type", Value: objectType}},
				bson.D{
					{
						Key: "approved_at",
						Value: bson.M{
							"$lte": primitive.NewDateTimeFromTime(time.Now()),
						},
					},
				},
			},
		},
	}

	c, err := r.collection.CountDocuments(ctx, filter, nil)
	if err != nil {
		return uint(c), err
	}

	return uint(c), nil
}
