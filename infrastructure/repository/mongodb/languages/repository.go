package languages

import (
	"context"
	"errors"
	"strings"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"

	"github.com/khanzadimahdi/testproject/domain"
	"github.com/khanzadimahdi/testproject/domain/language"
)

const (
	collectionName = "languages"
	queryTimeout   = 3 * time.Second
)

type LanguagesRepository struct {
	collection *mongo.Collection
}

var _ language.Repository = &LanguagesRepository{}

func NewRepository(database *mongo.Database) *LanguagesRepository {
	if database == nil {
		panic("database should not be nil")
	}

	return &LanguagesRepository{
		collection: database.Collection(collectionName),
	}
}

func (r *LanguagesRepository) GetAll(offset uint, limit uint) ([]language.Language, error) {
	ctx, cancel := context.WithTimeout(context.Background(), queryTimeout)
	defer cancel()

	o := int64(offset)
	l := int64(limit)
	asc := bson.D{{Key: "_id", Value: 1}}

	cur, err := r.collection.Find(ctx, bson.D{}, options.Find().SetSkip(o).SetLimit(l).SetSort(asc))
	if err != nil {
		return nil, err
	}
	defer cur.Close(ctx)

	items := make([]language.Language, 0, limit)
	for cur.Next(ctx) {
		var lb LanguageBson

		if err := cur.Decode(&lb); err != nil {
			return nil, err
		}
		items = append(items, language.Language{
			Code: lb.Code,
			Name: lb.Name,
		})
	}

	if err := cur.Err(); err != nil {
		return nil, err
	}

	return items, nil
}

func (r *LanguagesRepository) GetByCodes(codes []string) ([]language.Language, error) {
	ctx, cancel := context.WithTimeout(context.Background(), queryTimeout)
	defer cancel()

	if len(codes) == 0 {
		return []language.Language{}, nil
	}

	filter := bson.D{{Key: "_id", Value: bson.D{{Key: "$in", Value: codes}}}}

	cur, err := r.collection.Find(ctx, filter)
	if err != nil {
		return nil, err
	}
	defer cur.Close(ctx)

	items := make([]language.Language, 0, len(codes))
	for cur.Next(ctx) {
		var lb LanguageBson

		if err := cur.Decode(&lb); err != nil {
			return nil, err
		}
		items = append(items, language.Language{
			Code: lb.Code,
			Name: lb.Name,
		})
	}

	if err := cur.Err(); err != nil {
		return nil, err
	}

	return items, nil
}

func (r *LanguagesRepository) GetOne(code string) (language.Language, error) {
	ctx, cancel := context.WithTimeout(context.Background(), queryTimeout)
	defer cancel()

	var lb LanguageBson
	if err := r.collection.FindOne(ctx, bson.D{{Key: "_id", Value: code}}).Decode(&lb); err != nil {
		if errors.Is(err, mongo.ErrNoDocuments) {
			err = domain.ErrNotExists
		}
		return language.Language{}, err
	}

	return language.Language{
		Code: lb.Code,
		Name: lb.Name,
	}, nil
}

func (r *LanguagesRepository) Exists(code string) bool {
	ctx, cancel := context.WithTimeout(context.Background(), queryTimeout)
	defer cancel()

	c, err := r.collection.CountDocuments(ctx, bson.D{{Key: "_id", Value: code}}, options.Count().SetLimit(1))
	if err != nil {
		return false
	}

	return c > 0
}

func (r *LanguagesRepository) Save(l *language.Language) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), queryTimeout)
	defer cancel()

	at := time.Now()

	update := LanguageBson{
		Name:      l.Name,
		UpdatedAt: at,
	}

	// language code should be always lowercase
	l.Code = strings.ToLower(l.Code)

	_, err := r.collection.UpdateOne(
		ctx,
		bson.D{{Key: "_id", Value: l.Code}},
		bson.M{
			"$set":         bson.M{"name": update.Name, "updated_at": update.UpdatedAt},
			"$setOnInsert": bson.M{"created_at": at},
		},
		options.UpdateOne().SetUpsert(true),
	)

	return l.Code, err
}

func (r *LanguagesRepository) Delete(code string) error {
	ctx, cancel := context.WithTimeout(context.Background(), queryTimeout)
	defer cancel()

	_, err := r.collection.DeleteOne(ctx, bson.D{{Key: "_id", Value: code}})

	return err
}

func (r *LanguagesRepository) Count() (uint, error) {
	ctx, cancel := context.WithTimeout(context.Background(), queryTimeout)
	defer cancel()

	c, err := r.collection.CountDocuments(ctx, bson.D{})
	if err != nil {
		return uint(c), err
	}

	return uint(c), nil
}
