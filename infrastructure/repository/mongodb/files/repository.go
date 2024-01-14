package files

import (
	"context"
	"errors"
	"time"

	"github.com/gofrs/uuid/v5"
	"github.com/khanzadimahdi/testproject/domain"
	"github.com/khanzadimahdi/testproject/domain/file"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

const (
	collectionName = "files"
	queryTimeout   = 3 * time.Second
)

type FilesRepository struct {
	collection *mongo.Collection
}

var _ file.Repository = &FilesRepository{}

func NewFilesRepository(database *mongo.Database) *FilesRepository {
	if database == nil {
		panic("database should not be nil")
	}

	return &FilesRepository{
		collection: database.Collection(collectionName),
	}
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
