package file

import (
	"context"
	"io"

	"github.com/minio/minio-go/v7"
)

type File struct {
	UUID      string
	Name      string
	Size      int64
	OwnerUUID string
}

type Repository interface {
	GetOne(UUID string) (File, error)
	Save(*File) error
	Delete(UUID string) error
}

type Storage interface {
	Store(ctx context.Context, objectName string, reader io.Reader, objectSize int64) error
	Delete(ctx context.Context, objectName string) error
	Read(ctx context.Context, objectName string) (*minio.Object, error)
}
