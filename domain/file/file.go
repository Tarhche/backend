package file

import (
	"context"
	"io"
	"time"
)

type File struct {
	UUID       string
	Name       string
	StoredName string
	Size       int64
	OwnerUUID  string
	MimeType   string
	CreatedAt  time.Time
}

type Repository interface {
	GetAll(ctx context.Context, offset uint, limit uint) ([]File, error)
	GetOne(ctx context.Context, UUID string) (File, error)
	Save(ctx context.Context, f *File) (string, error)
	Delete(ctx context.Context, UUID string) error
	Count(ctx context.Context) (uint, error)

	GetAllByOwnerUUID(ctx context.Context, ownerUUID string, offset uint, limit uint) ([]File, error)
	GetOneByOwnerUUID(ctx context.Context, ownerUUID string, UUID string) (File, error)
	DeleteByOwnerUUID(ctx context.Context, ownerUUID string, UUID string) error
	CountByOwnerUUID(ctx context.Context, ownerUUID string) (uint, error)
}

type Storage interface {
	Store(ctx context.Context, objectName string, reader io.Reader, objectSize int64) error
	Delete(ctx context.Context, objectName string) error
	Read(ctx context.Context, objectName string) (io.ReadSeekCloser, error)
}
