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

// EnsureIndexes lets the database throw away a registration nobody ever
// approved. Anybody may register, and what is never used should not be kept
// for ever: a client is read by its id alone, so expiry is the only index this
// collection needs.
func (r *ClientsRepository) EnsureIndexes(ctx context.Context) error {
	ctx, cancel := context.WithTimeout(ctx, queryTimeout)
	defer cancel()

	_, err := r.collection.Indexes().CreateOne(ctx, mongo.IndexModel{
		Keys:    bson.D{{Key: "expired_at", Value: 1}},
		Options: options.Index().SetExpireAfterSeconds(0),
	})

	return err
}

// Keep stops a client from expiring, which is what approving one means. A
// document with no expiry is one the database has nothing to say about.
func (r *ClientsRepository) Keep(ctx context.Context, id string) error {
	ctx, cancel := context.WithTimeout(ctx, queryTimeout)
	defer cancel()

	result, err := r.collection.UpdateOne(
		ctx,
		bson.D{{Key: "_id", Value: id}},
		bson.M{"$unset": bson.M{"expired_at": ""}},
	)
	if err != nil {
		return err
	}

	if result.MatchedCount == 0 {
		return domain.ErrNotExists
	}

	return nil
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

	var expiredAt *time.Time
	if !c.ExpiredAt.IsZero() {
		expiredAt = &c.ExpiredAt
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
		ExpiredAt: expiredAt,
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

	c := client.Client{
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
	}

	if stored.ExpiredAt != nil {
		c.ExpiredAt = *stored.ExpiredAt
	}

	return c, nil
}
