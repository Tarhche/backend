package roles

import (
	"context"
	"errors"
	"time"

	"github.com/gofrs/uuid/v5"
	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"

	"github.com/khanzadimahdi/testproject/domain"
	"github.com/khanzadimahdi/testproject/domain/config"
)

const (
	collectionName = "config"
	queryTimeout   = 3 * time.Second
)

type ConfigRepository struct {
	collection *mongo.Collection
}

var _ config.Repository = &ConfigRepository{}

func NewRepository(database *mongo.Database) *ConfigRepository {
	if database == nil {
		panic("database should not be nil")
	}

	return &ConfigRepository{
		collection: database.Collection(collectionName),
	}
}

func (r *ConfigRepository) GetLatestRevision() (config.Config, error) {
	ctx, cancel := context.WithTimeout(context.Background(), queryTimeout)
	defer cancel()

	desc := bson.D{{Key: "_id", Value: -1}}
	sort := options.FindOne().SetSort(desc)

	var c configBson
	if err := r.collection.FindOne(ctx, bson.D{}, sort).Decode(&c); err != nil {
		if errors.Is(err, mongo.ErrNoDocuments) {
			err = domain.ErrNotExists
		}
		return config.Config{}, err
	}

	return config.Config{
		Revision:             c.Revision,
		UserDefaultRoleUUIDs: c.UserDefaultRoleUUIDs,
	}, nil
}

func (r *ConfigRepository) Save(a *config.Config) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), queryTimeout)
	defer cancel()

	UUID, err := uuid.NewV7()
	if err != nil {
		return "", err
	}

	record := configBson{
		UUID:                 UUID.String(),
		Revision:             a.Revision + 1,
		UserDefaultRoleUUIDs: a.UserDefaultRoleUUIDs,
		CreatedAt:            time.Now(),
	}

	_, err = r.collection.InsertOne(ctx, record)

	return record.UUID, err
}
