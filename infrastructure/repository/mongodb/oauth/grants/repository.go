package grants

import (
	"context"
	"errors"
	"time"

	"github.com/gofrs/uuid/v5"
	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"

	"github.com/khanzadimahdi/testproject/domain"
	"github.com/khanzadimahdi/testproject/domain/oauth/grant"
	"github.com/khanzadimahdi/testproject/domain/password"
)

const (
	collectionName = "oauth_grants"
	queryTimeout   = 3 * time.Second
)

// GrantsRepository keeps the codes handed out to applications somebody
// approved, for the moment it takes them to be collected.
type GrantsRepository struct {
	collection *mongo.Collection
}

var _ grant.Repository = &GrantsRepository{}

func NewRepository(database *mongo.Database) *GrantsRepository {
	if database == nil {
		panic("database should not be nil")
	}

	return &GrantsRepository{
		collection: database.Collection(collectionName),
	}
}

// EnsureIndexes lets the database throw away what was never collected. A grant
// is read once, by its id, so expiry is the only index the collection needs.
func (r *GrantsRepository) EnsureIndexes(ctx context.Context) error {
	ctx, cancel := context.WithTimeout(ctx, queryTimeout)
	defer cancel()

	_, err := r.collection.Indexes().CreateOne(ctx, mongo.IndexModel{
		Keys:    bson.D{{Key: "expired_at", Value: 1}},
		Options: options.Index().SetExpireAfterSeconds(0),
	})

	return err
}

func (r *GrantsRepository) Save(ctx context.Context, g *grant.Grant) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, queryTimeout)
	defer cancel()

	if len(g.ID) == 0 {
		ID, err := uuid.NewV7()
		if err != nil {
			return "", err
		}
		g.ID = ID.String()
	}

	if g.CreatedAt.IsZero() {
		g.CreatedAt = time.Now()
	}

	if _, err := r.collection.InsertOne(ctx, GrantBson{
		ID: g.ID,
		Secret: SecretBson{
			Value: g.Secret.Value,
			Salt:  g.Secret.Salt,
		},
		ClientID:            g.ClientID,
		UserUUID:            g.UserUUID,
		RedirectURI:         g.RedirectURI,
		Scope:               g.Scope,
		CodeChallenge:       g.CodeChallenge,
		CodeChallengeMethod: g.CodeChallengeMethod,
		ExpiredAt:           g.ExpiredAt,
		CreatedAt:           g.CreatedAt,
	}); err != nil {
		return "", err
	}

	return g.ID, nil
}

// Consume reads a grant and deletes it in the same operation, so a code that
// is presented twice is exchanged at most once.
func (r *GrantsRepository) Consume(ctx context.Context, id string) (grant.Grant, error) {
	ctx, cancel := context.WithTimeout(ctx, queryTimeout)
	defer cancel()

	var stored GrantBson
	if err := r.collection.FindOneAndDelete(ctx, bson.D{{Key: "_id", Value: id}}).Decode(&stored); err != nil {
		if errors.Is(err, mongo.ErrNoDocuments) {
			err = domain.ErrNotExists
		}

		return grant.Grant{}, err
	}

	return grant.Grant{
		ID: stored.ID,
		Secret: password.Hash{
			Value: stored.Secret.Value,
			Salt:  stored.Secret.Salt,
		},
		ClientID:            stored.ClientID,
		UserUUID:            stored.UserUUID,
		RedirectURI:         stored.RedirectURI,
		Scope:               stored.Scope,
		CodeChallenge:       stored.CodeChallenge,
		CodeChallengeMethod: stored.CodeChallengeMethod,
		ExpiredAt:           stored.ExpiredAt,
		CreatedAt:           stored.CreatedAt,
	}, nil
}
