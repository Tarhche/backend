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
}

type Repository interface {
	GetAll(offset uint, limit uint) ([]File, error)
	GetOne(UUID string) (File, error)
	Save(*File) (string, error)
	Delete(UUID string) error
	Count() (uint, error)
}

type Storage interface {
	Store(ctx context.Context, objectName string, reader io.Reader, objectSize int64) error
	Delete(ctx context.Context, objectName string) error
	Read(ctx context.Context, objectName string) (io.ReadCloser, error)
}
