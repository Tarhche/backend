package contacts

import (
	"context"
	"errors"
	"time"

	"github.com/gofrs/uuid/v5"
	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"

	"github.com/khanzadimahdi/testproject/domain"
	"github.com/khanzadimahdi/testproject/domain/contact"
)

const (
	collectionName = "contact_messages"
	queryTimeout   = 3 * time.Second
)

type ContactsRepository struct {
	collection *mongo.Collection
}

var _ contact.Repository = &ContactsRepository{}

func NewRepository(database *mongo.Database) *ContactsRepository {
	if database == nil {
		panic("database should not be nil")
	}

	return &ContactsRepository{
		collection: database.Collection(collectionName),
	}
}

func (r *ContactsRepository) GetAll(ctx context.Context, offset uint, limit uint) ([]contact.Message, error) {
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

	items := make([]contact.Message, 0, limit)
	for cur.Next(ctx) {
		var m ContactMessageBson

		if err := cur.Decode(&m); err != nil {
			return nil, err
		}
		items = append(items, contact.Message{
			UUID:      m.UUID,
			Subject:   m.Subject,
			Body:      m.Body,
			Email:     m.Email,
			Phone:     m.Phone,
			ReadAt:    m.ReadAt,
			CreatedAt: m.CreatedAt,
		})
	}

	if err := cur.Err(); err != nil {
		return nil, err
	}

	return items, nil
}

func (r *ContactsRepository) GetOne(ctx context.Context, UUID string) (contact.Message, error) {
	ctx, cancel := context.WithTimeout(ctx, queryTimeout)
	defer cancel()

	filter := bson.D{{Key: "_id", Value: UUID}}

	var m ContactMessageBson
	if err := r.collection.FindOne(ctx, filter).Decode(&m); err != nil {
		if errors.Is(err, mongo.ErrNoDocuments) {
			err = domain.ErrNotExists
		}
		return contact.Message{}, err
	}

	return contact.Message{
		UUID:      m.UUID,
		Subject:   m.Subject,
		Body:      m.Body,
		Email:     m.Email,
		Phone:     m.Phone,
		ReadAt:    m.ReadAt,
		CreatedAt: m.CreatedAt,
	}, nil
}

func (r *ContactsRepository) Count(ctx context.Context) (uint, error) {
	ctx, cancel := context.WithTimeout(ctx, queryTimeout)
	defer cancel()

	c, err := r.collection.CountDocuments(ctx, bson.D{})
	if err != nil {
		return uint(c), err
	}

	return uint(c), nil
}

func (r *ContactsRepository) Save(ctx context.Context, m *contact.Message) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, queryTimeout)
	defer cancel()

	if len(m.UUID) == 0 {
		UUID, err := uuid.NewV7()
		if err != nil {
			return "", err
		}
		m.UUID = UUID.String()
		m.CreatedAt = time.Now()
	}

	update := ContactMessageBson{
		UUID:      m.UUID,
		Subject:   m.Subject,
		Body:      m.Body,
		Email:     m.Email,
		Phone:     m.Phone,
		ReadAt:    m.ReadAt,
		CreatedAt: m.CreatedAt,
		UpdatedAt: time.Now(),
	}

	if _, err := r.collection.UpdateOne(
		ctx,
		bson.D{{Key: "_id", Value: m.UUID}},
		bson.M{"$set": update},
		options.UpdateOne().SetUpsert(true),
	); err != nil {
		return "", err
	}

	return m.UUID, nil
}

func (r *ContactsRepository) Delete(ctx context.Context, UUID string) error {
	ctx, cancel := context.WithTimeout(ctx, queryTimeout)
	defer cancel()

	_, err := r.collection.DeleteOne(ctx, bson.D{{Key: "_id", Value: UUID}})

	return err
}
