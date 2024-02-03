package elements

import (
	"context"
	"errors"
	"time"

	"github.com/gofrs/uuid/v5"
	"github.com/khanzadimahdi/testproject/domain"
	"github.com/khanzadimahdi/testproject/domain/element"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

const (
	collectionName = "elements"
	queryTimeout   = 3 * time.Second
)

type ElementsRepository struct {
	collection *mongo.Collection
}

var _ element.Repository = &ElementsRepository{}

func NewElementsRepository(database *mongo.Database) *ElementsRepository {
	if database == nil {
		panic("database should not be nil")
	}

	return &ElementsRepository{
		collection: database.Collection(collectionName),
	}
}

func (r *ElementsRepository) GetAll(offset uint, limit uint) ([]element.Element, error) {
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

	items := make([]element.Element, 0, limit)
	for cur.Next(ctx) {
		var a ElementBson

		if err := cur.Decode(&a); err != nil {
			return nil, err
		}
		items = append(items, element.Element{
			UUID:      a.UUID,
			Type:      a.Type,
			Body:      a.Body,
			Venues:    a.Venues,
			CreatedAt: a.CreatedAt,
			UpdatedAt: a.UpdatedAt,
		})
	}

	if err := cur.Err(); err != nil {
		return nil, err
	}

	return items, nil
}

func (r *ElementsRepository) GetByVenues(venues []string) ([]element.Element, error) {
	ctx, cancel := context.WithTimeout(context.Background(), queryTimeout)
	defer cancel()

	filter := bson.M{"tags": bson.M{"$in": venues}}
	cur, err := r.collection.Find(ctx, filter, &options.FindOptions{})

	if err != nil {
		return nil, err
	}

	items := make([]element.Element, 0, 2)
	for cur.Next(ctx) {
		var a ElementBson

		if err := cur.Decode(&a); err != nil {
			return nil, err
		}
		items = append(items, element.Element{
			UUID:      a.UUID,
			Type:      a.Type,
			Body:      a.Body,
			Venues:    a.Venues,
			CreatedAt: a.CreatedAt,
			UpdatedAt: a.UpdatedAt,
		})
	}

	if err := cur.Err(); err != nil {
		return nil, err
	}

	return items, nil
}

func (r *ElementsRepository) GetOne(UUID string) (element.Element, error) {
	ctx, cancel := context.WithTimeout(context.Background(), queryTimeout)
	defer cancel()

	var a ElementBson
	if err := r.collection.FindOne(ctx, bson.D{{Key: "_id", Value: UUID}}, nil).Decode(&a); err != nil {
		if errors.Is(err, mongo.ErrNoDocuments) {
			err = domain.ErrNotExists
		}
		return element.Element{}, err
	}

	return element.Element{
		UUID:      a.UUID,
		Type:      a.Type,
		Body:      a.Body,
		Venues:    a.Venues,
		CreatedAt: a.CreatedAt,
		UpdatedAt: a.UpdatedAt,
	}, nil
}

func (r *ElementsRepository) Count() (uint, error) {
	ctx, cancel := context.WithTimeout(context.Background(), queryTimeout)
	defer cancel()

	c, err := r.collection.CountDocuments(ctx, bson.D{}, nil)
	if err != nil {
		return uint(c), err
	}

	return uint(c), nil
}

func (r *ElementsRepository) Save(a *element.Element) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), queryTimeout)
	defer cancel()

	if len(a.UUID) == 0 {
		UUID, err := uuid.NewV7()
		if err != nil {
			return "", err
		}
		a.UUID = UUID.String()
	}

	now := time.Now()
	if a.CreatedAt.IsZero() {
		a.CreatedAt = now
	}

	update := ElementBson{
		UUID:      a.UUID,
		Type:      a.Type,
		Body:      a.Body,
		Venues:    a.Venues,
		CreatedAt: a.CreatedAt,
		UpdatedAt: now,
	}

	upsert := true
	_, err := r.collection.UpdateOne(
		ctx,
		bson.D{{Key: "_id", Value: a.UUID}},
		SetWrapper{Set: update},
		&options.UpdateOptions{Upsert: &upsert},
	)
	if err != nil {
		return "", err
	}

	return a.UUID, nil
}

func (r *ElementsRepository) Delete(UUID string) error {
	ctx, cancel := context.WithTimeout(context.Background(), queryTimeout)
	defer cancel()

	_, err := r.collection.DeleteOne(ctx, bson.D{{Key: "_id", Value: UUID}}, nil)

	return err
}
