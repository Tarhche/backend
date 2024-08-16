package roles

import (
	"context"
	"errors"
	"time"

	"github.com/gofrs/uuid/v5"
	"github.com/khanzadimahdi/testproject/domain"
	"github.com/khanzadimahdi/testproject/domain/role"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

const (
	collectionName = "roles"
	queryTimeout   = 3 * time.Second
)

type RolesRepository struct {
	collection *mongo.Collection
}

var _ role.Repository = &RolesRepository{}

func NewRepository(database *mongo.Database) *RolesRepository {
	if database == nil {
		panic("database should not be nil")
	}

	return &RolesRepository{
		collection: database.Collection(collectionName),
	}
}

func (r *RolesRepository) GetAll(offset uint, limit uint) ([]role.Role, error) {
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

	items := make([]role.Role, 0, limit)
	for cur.Next(ctx) {
		var r RoleBson

		if err := cur.Decode(&r); err != nil {
			return nil, err
		}
		items = append(items, role.Role{
			UUID:        r.UUID,
			Name:        r.Name,
			Description: r.Description,
			Permissions: r.Permissions,
		})
	}

	if err := cur.Err(); err != nil {
		return nil, err
	}

	return items, nil
}

func (r *RolesRepository) GetOne(UUID string) (role.Role, error) {
	ctx, cancel := context.WithTimeout(context.Background(), queryTimeout)
	defer cancel()

	var rb RoleBson
	if err := r.collection.FindOne(ctx, bson.D{{Key: "_id", Value: UUID}}, nil).Decode(&rb); err != nil {
		if errors.Is(err, mongo.ErrNoDocuments) {
			err = domain.ErrNotExists
		}
		return role.Role{}, err
	}

	return role.Role{
		UUID:        rb.UUID,
		Name:        rb.Name,
		Description: rb.Description,
		Permissions: rb.Permissions,
		UserUUIDs:   rb.UserUUIDs,
	}, nil
}

func (r *RolesRepository) Save(a *role.Role) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), queryTimeout)
	defer cancel()

	at := time.Now()

	update := RoleBson{
		Name:        a.Name,
		Description: a.Description,
		Permissions: a.Permissions,
		UserUUIDs:   a.UserUUIDs,
		UpdatedAt:   at,
	}

	if len(a.UUID) == 0 {
		UUID, err := uuid.NewV7()
		if err != nil {
			return "", err
		}
		a.UUID = UUID.String()

		update.UUID = a.UUID
		update.CreatedAt = at
	}

	upsert := true
	_, err := r.collection.UpdateOne(ctx, bson.D{{Key: "_id", Value: a.UUID}}, SetWrapper{Set: update}, &options.UpdateOptions{
		Upsert: &upsert,
	})

	return a.UUID, err
}

func (r *RolesRepository) Count() (uint, error) {
	ctx, cancel := context.WithTimeout(context.Background(), queryTimeout)
	defer cancel()

	c, err := r.collection.CountDocuments(ctx, bson.D{}, nil)
	if err != nil {
		return uint(c), err
	}

	return uint(c), nil
}

func (r *RolesRepository) Delete(UUID string) error {
	ctx, cancel := context.WithTimeout(context.Background(), queryTimeout)
	defer cancel()

	_, err := r.collection.DeleteOne(ctx, bson.D{{Key: "_id", Value: UUID}}, nil)

	return err
}

func (r *RolesRepository) UserHasPermission(userUUID string, permission string) (bool, error) {
	ctx, cancel := context.WithTimeout(context.Background(), queryTimeout)
	defer cancel()

	filter := bson.D{
		{
			Key: "$and",
			Value: bson.A{
				bson.D{{Key: "user_uuids", Value: userUUID}},
				bson.D{{Key: "permissions", Value: permission}},
			},
		},
	}

	c, err := r.collection.CountDocuments(ctx, filter, nil)
	if err != nil {
		return c > 0, err
	}

	return c > 0, nil
}

func (r *RolesRepository) GetByUserUUID(userUUID string) ([]role.Role, error) {
	ctx, cancel := context.WithTimeout(context.Background(), queryTimeout)
	defer cancel()

	filter := bson.D{{Key: "user_uuids", Value: userUUID}}

	cur, err := r.collection.Find(ctx, filter, nil)

	if err != nil {
		return nil, err
	}

	defer cur.Close(ctx)

	items := make([]role.Role, 0, 2)
	for cur.Next(ctx) {
		var r RoleBson

		if err := cur.Decode(&r); err != nil {
			return nil, err
		}
		items = append(items, role.Role{
			UUID:        r.UUID,
			Name:        r.Name,
			Description: r.Description,
			Permissions: r.Permissions,
		})
	}

	if err := cur.Err(); err != nil {
		return nil, err
	}

	return items, nil
}
