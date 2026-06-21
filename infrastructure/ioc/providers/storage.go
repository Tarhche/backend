package providers

import (
	"context"
	"os"
	"strconv"

	"github.com/danceable/container/bind"
	"github.com/danceable/provider"

	"github.com/khanzadimahdi/testproject/domain/file"
	"github.com/khanzadimahdi/testproject/infrastructure/storage/minio"
)

type storageProvider struct{}

var _ provider.Provider = &storageProvider{}

func NewStorageProvider() *storageProvider {
	return &storageProvider{}
}

func (p *storageProvider) Register(ctx context.Context, c provider.Container) error {
	useSSL, err := strconv.ParseBool(os.Getenv("S3_USE_SSL"))
	if err != nil {
		return err
	}

	fileStorage, err := minio.New(minio.Options{
		Endpoint:   os.Getenv("S3_ENDPOINT"),
		AccessKey:  os.Getenv("S3_ACCESS_KEY"),
		SecretKey:  os.Getenv("S3_SECRET_KEY"),
		UseSSL:     useSSL,
		BucketName: os.Getenv("S3_BUCKET_NAME"),
	})
	if err != nil {
		return err
	}

	return c.Bind(func() file.Storage { return fileStorage }, bind.Singleton())
}

func (p *storageProvider) Boot(ctx context.Context, c provider.Container) error {
	return nil
}

func (p *storageProvider) Terminate(ctx context.Context) error {
	return nil
}
