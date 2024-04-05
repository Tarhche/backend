package users

import (
	"context"
	"errors"
	"time"

	"github.com/gofrs/uuid/v5"
	"github.com/khanzadimahdi/testproject/domain"
	"github.com/khanzadimahdi/testproject/domain/password"
	"github.com/khanzadimahdi/testproject/domain/user"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

const (
	collectionName = "users"
	queryTimeout   = 3 * time.Second
)

type UsersRepository struct {
	collection *mongo.Collection
}

var _ user.Repository = &UsersRepository{}

func NewUsersRepository(database *mongo.Database) *UsersRepository {
	if database == nil {
		panic("database should not be nil")
	}

	return &UsersRepository{
		collection: database.Collection(collectionName),
	}
}

func (r *UsersRepository) GetOne(UUID string) (user.User, error) {
	ctx, cancel := context.WithTimeout(context.Background(), queryTimeout)
	defer cancel()

	var a UserBson
	if err := r.collection.FindOne(ctx, bson.D{{Key: "_id", Value: UUID}}, nil).Decode(&a); err != nil {
		if errors.Is(err, mongo.ErrNoDocuments) {
			err = domain.ErrNotExists
		}
		return user.User{}, err
	}

	return user.User{
		UUID:     a.UUID,
		Name:     a.Name,
		Avatar:   a.Avatar,
		Email:    a.Email,
		Username: a.Username,
		PasswordHash: password.Hash{
			Value: a.PasswordHash.Value,
			Salt:  a.PasswordHash.Salt,
		},
	}, nil
}

// GetOneByIdentity returns a user which its email or username matches given identity
func (r *UsersRepository) GetOneByIdentity(identity string) (user.User, error) {
	ctx, cancel := context.WithTimeout(context.Background(), queryTimeout)
	defer cancel()

	filter := bson.D{
		{
			Key: "$or",
			Value: bson.A{
				bson.D{{Key: "email", Value: identity}},
				bson.D{{Key: "username", Value: identity}},
			},
		},
	}

	var a UserBson
	if err := r.collection.FindOne(ctx, filter, nil).Decode(&a); err != nil {
		if errors.Is(err, mongo.ErrNoDocuments) {
			err = domain.ErrNotExists
		}
		return user.User{}, err
	}

	return user.User{
		UUID:     a.UUID,
		Name:     a.Name,
		Avatar:   a.Avatar,
		Email:    a.Email,
		Username: a.Username,
		PasswordHash: password.Hash{
			Value: a.PasswordHash.Value,
			Salt:  a.PasswordHash.Salt,
		},
	}, nil
}

func (r *UsersRepository) Save(a *user.User) error {
	ctx, cancel := context.WithTimeout(context.Background(), queryTimeout)
	defer cancel()

	if len(a.UUID) == 0 {
		UUID, err := uuid.NewV7()
		if err != nil {
			return err
		}
		a.UUID = UUID.String()
	}

	update := UserBson{
		UUID:     a.UUID,
		Name:     a.Name,
		Avatar:   a.Avatar,
		Email:    a.Email,
		Username: a.Username,
		PasswordHash: PasswordHashBson{
			Value: a.PasswordHash.Value,
			Salt:  a.PasswordHash.Salt,
		},
		CreatedAt: time.Now(),
	}

	upsert := true
	_, err := r.collection.UpdateOne(ctx, bson.D{{Key: "_id", Value: a.UUID}}, SetWrapper{Set: update}, &options.UpdateOptions{
		Upsert: &upsert,
	})

	return err
}
