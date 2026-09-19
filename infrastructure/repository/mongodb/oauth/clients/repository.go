package clients

import (
	"context"
	"errors"
	"time"

	"github.com/gofrs/uuid/v5"
	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"

	"github.com/khanzadimahdi/testproject/domain"
	"github.com/khanzadimahdi/testproject/domain/oauth/client"
	"github.com/khanzadimahdi/testproject/domain/password"
)

const (
	collectionName = "oauth_clients"
	queryTimeout   = 3 * time.Second
)

type ClientsRepository struct {
	collection *mongo.Collection
}

var _ client.Repository = &ClientsRepository{}

func NewRepository(database *mongo.Database) *ClientsRepository {
	if database == nil {
		panic("database should not be nil")
	}

	return &ClientsRepository{
		collection: database.Collection(collectionName),
	}
}

func (r *ClientsRepository) Save(ctx context.Context, c *client.Client) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, queryTimeout)
	defer cancel()

	if len(c.ID) == 0 {
		ID, err := uuid.NewV7()
		if err != nil {
			return "", err
		}
		c.ID = ID.String()
	}

	if c.CreatedAt.IsZero() {
		c.CreatedAt = time.Now()
	}

	stored := ClientBson{
		ID:                      c.ID,
		Name:                    c.Name,
		URI:                     c.URI,
		RedirectURIs:            c.RedirectURIs,
		GrantTypes:              c.GrantTypes,
		ResponseTypes:           c.ResponseTypes,
		TokenEndpointAuthMethod: c.TokenEndpointAuthMethod,
		Scope:                   c.Scope,
		Secret: SecretBson{
			Value: c.Secret.Value,
			Salt:  c.Secret.Salt,
		},
		CreatedAt: c.CreatedAt,
	}

	update := bson.M{"$set": stored}
	if _, err := r.collection.UpdateOne(
		ctx,
		bson.D{{Key: "_id", Value: c.ID}},
		update,
		options.UpdateOne().SetUpsert(true),
	); err != nil {
		return "", err
	}

	return c.ID, nil
}

func (r *ClientsRepository) GetOne(ctx context.Context, id string) (client.Client, error) {
	ctx, cancel := context.WithTimeout(ctx, queryTimeout)
	defer cancel()

	var stored ClientBson
	if err := r.collection.FindOne(ctx, bson.D{{Key: "_id", Value: id}}).Decode(&stored); err != nil {
		if errors.Is(err, mongo.ErrNoDocuments) {
			err = domain.ErrNotExists
		}

		return client.Client{}, err
	}

	return client.Client{
		ID:                      stored.ID,
		Name:                    stored.Name,
		URI:                     stored.URI,
		RedirectURIs:            stored.RedirectURIs,
		GrantTypes:              stored.GrantTypes,
		ResponseTypes:           stored.ResponseTypes,
		TokenEndpointAuthMethod: stored.TokenEndpointAuthMethod,
		Scope:                   stored.Scope,
		Secret: password.Hash{
			Value: stored.Secret.Value,
			Salt:  stored.Secret.Salt,
		},
		CreatedAt: stored.CreatedAt,
	}, nil
}
