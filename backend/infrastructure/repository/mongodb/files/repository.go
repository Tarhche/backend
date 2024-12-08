package files

import (
	"context"
	"errors"
	"time"

	"github.com/gofrs/uuid/v5"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"

	"github.com/khanzadimahdi/testproject/domain"
	"github.com/khanzadimahdi/testproject/domain/file"
)

const (
	collectionName = "files"
	queryTimeout   = 3 * time.Second
)

type FilesRepository struct {
	collection *mongo.Collection
}

var _ file.Repository = &FilesRepository{}

func NewRepository(database *mongo.Database) *FilesRepository {
	if database == nil {
		panic("database should not be nil")
	}

	return &FilesRepository{
		collection: database.Collection(collectionName),
	}
}

func (r *FilesRepository) GetAll(offset uint, limit uint) ([]file.File, error) {
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

	items := make([]file.File, 0, limit)
	for cur.Next(ctx) {
		var a FileBson

		if err := cur.Decode(&a); err != nil {
			return nil, err
		}
		items = append(items, file.File{
			UUID:      a.UUID,
			Name:      a.Name,
			Size:      a.Size,
			OwnerUUID: a.OwnerUUID,
			MimeType:  a.MimeType,
			CreatedAt: a.CreatedAt,
		})
	}

	if err := cur.Err(); err != nil {
		return nil, err
	}

	return items, nil
}

func (r *FilesRepository) GetOne(UUID string) (file.File, error) {
	ctx, cancel := context.WithTimeout(context.Background(), queryTimeout)
	defer cancel()

	var a FileBson
	if err := r.collection.FindOne(ctx, bson.D{{Key: "_id", Value: UUID}}, nil).Decode(&a); err != nil {
		if errors.Is(err, mongo.ErrNoDocuments) {
			err = domain.ErrNotExists
		}
		return file.File{}, err
	}

	return file.File{
		UUID:      a.UUID,
		Name:      a.Name,
		Size:      a.Size,
		OwnerUUID: a.OwnerUUID,
		MimeType:  a.MimeType,
		CreatedAt: a.CreatedAt,
	}, nil
}

func (r *FilesRepository) Save(a *file.File) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), queryTimeout)
	defer cancel()

	if len(a.UUID) == 0 {
		UUID, err := uuid.NewV7()
		if err != nil {
			return "", err
		}
		a.UUID = UUID.String()
	}

	update := FileBson{
		UUID:      a.UUID,
		Name:      a.Name,
		Size:      a.Size,
		OwnerUUID: a.OwnerUUID,
		MimeType:  a.MimeType,
		CreatedAt: time.Now(),
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

func (r *FilesRepository) Delete(UUID string) error {
	ctx, cancel := context.WithTimeout(context.Background(), queryTimeout)
	defer cancel()

	_, err := r.collection.DeleteOne(ctx, bson.D{{Key: "_id", Value: UUID}}, nil)

	return err
}

func (r *FilesRepository) Count() (uint, error) {
	ctx, cancel := context.WithTimeout(context.Background(), queryTimeout)
	defer cancel()

	c, err := r.collection.CountDocuments(ctx, bson.D{}, nil)
	if err != nil {
		return uint(c), err
	}

	return uint(c), nil
}

func (r *FilesRepository) GetAllByOwnerUUID(ownerUUID string, offset uint, limit uint) ([]file.File, error) {
	ctx, cancel := context.WithTimeout(context.Background(), queryTimeout)
	defer cancel()

	o := int64(offset)
	l := int64(limit)
	desc := bson.D{{Key: "_id", Value: -1}}
	filter := bson.M{
		"owner_uuid": ownerUUID,
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

	items := make([]file.File, 0, limit)
	for cur.Next(ctx) {
		var a FileBson

		if err := cur.Decode(&a); err != nil {
			return nil, err
		}
		items = append(items, file.File{
			UUID:      a.UUID,
			Name:      a.Name,
			Size:      a.Size,
			OwnerUUID: a.OwnerUUID,
			MimeType:  a.MimeType,
			CreatedAt: a.CreatedAt,
		})
	}

	if err := cur.Err(); err != nil {
		return nil, err
	}

	return items, nil
}

func (r *FilesRepository) GetOneByOwnerUUID(ownerUUID string, UUID string) (file.File, error) {
	ctx, cancel := context.WithTimeout(context.Background(), queryTimeout)
	defer cancel()

	filter := bson.D{
		{Key: "_id", Value: UUID},
		{Key: "owner_uuid", Value: ownerUUID},
	}

	var a FileBson
	if err := r.collection.FindOne(ctx, filter, nil).Decode(&a); err != nil {
		if errors.Is(err, mongo.ErrNoDocuments) {
			err = domain.ErrNotExists
		}
		return file.File{}, err
	}

	return file.File{
		UUID:      a.UUID,
		Name:      a.Name,
		Size:      a.Size,
		OwnerUUID: a.OwnerUUID,
		MimeType:  a.MimeType,
		CreatedAt: a.CreatedAt,
	}, nil
}

func (r *FilesRepository) DeleteByOwnerUUID(ownerUUID string, UUID string) error {
	ctx, cancel := context.WithTimeout(context.Background(), queryTimeout)
	defer cancel()

	filter := bson.D{
		{Key: "_id", Value: UUID},
		{Key: "owner_uuid", Value: ownerUUID},
	}

	_, err := r.collection.DeleteOne(ctx, filter, nil)

	return err
}

func (r *FilesRepository) CountByOwnerUUID(ownerUUID string) (uint, error) {
	ctx, cancel := context.WithTimeout(context.Background(), queryTimeout)
	defer cancel()

	filter := bson.M{
		"owner_uuid": ownerUUID,
	}

	c, err := r.collection.CountDocuments(ctx, filter, nil)
	if err != nil {
		return uint(c), err
	}

	return uint(c), nil
}
