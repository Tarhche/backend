package file

import (
	"context"
	"io"
)

type File struct {
	UUID      string
	Name      string
	Size      int64
	OwnerUUID string
	MimeType  string
}

type Repository interface {
	GetAll(offset uint, limit uint) ([]File, error)
	GetOne(UUID string) (File, error)
	Save(*File) (string, error)
	Delete(UUID string) error
	Count() (uint, error)

	GetAllByOwnerUUID(ownerUUID string, offset uint, limit uint) ([]File, error)
	GetOneByOwnerUUID(ownerUUID string, UUID string) (File, error)
	DeleteByOwnerUUID(ownerUUID string, UUID string) error
	CountByOwnerUUID(ownerUUID string) (uint, error)
}

type Storage interface {
	Store(ctx context.Context, objectName string, reader io.Reader, objectSize int64) error
	Delete(ctx context.Context, objectName string) error
	Read(ctx context.Context, objectName string) (io.ReadSeekCloser, error)
}
