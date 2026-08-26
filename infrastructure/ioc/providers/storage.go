package providers

import (
	"context"

	"github.com/danceable/provider"

	"github.com/khanzadimahdi/testproject/domain/file"
	"github.com/khanzadimahdi/testproject/infrastructure/configs"
	"github.com/khanzadimahdi/testproject/infrastructure/storage/minio"
)

type storageProvider struct{}

var _ provider.Provider = &storageProvider{}

func NewStorageProvider() *storageProvider {
	return &storageProvider{}
}

func (p *storageProvider) Register(ctx context.Context, c provider.Container) error {
	var blogConfigs *configs.Blog
	if err := c.Resolve(&blogConfigs); err != nil {
		return err
	}

	fileStorage, err := minio.New(minio.Options{
		Endpoint:   blogConfigs.S3Endpoint,
		AccessKey:  blogConfigs.S3AccessKey,
		SecretKey:  blogConfigs.S3SecretKey,
		UseSSL:     blogConfigs.S3UseSSL,
		BucketName: blogConfigs.S3BucketName,
	})
	if err != nil {
		return err
	}

	return c.Bind(func() file.Storage { return fileStorage }, provider.Singleton())
}

func (p *storageProvider) Boot(ctx context.Context, c provider.Container) error {
	return nil
}

func (p *storageProvider) Terminate(ctx context.Context) error {
	return nil
}
