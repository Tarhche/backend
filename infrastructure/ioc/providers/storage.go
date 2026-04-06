package providers

import (
	"os"
	"strconv"

	"github.com/khanzadimahdi/testproject/domain/file"
	"github.com/khanzadimahdi/testproject/infrastructure/ioc"
	"github.com/khanzadimahdi/testproject/infrastructure/storage/minio"
)

type storageProvider struct{}

var _ ioc.ServiceProvider = &storageProvider{}

func NewStorageProvider() *storageProvider {
	return &storageProvider{}
}

func (p *storageProvider) Register(app *ioc.Application) error {
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

	return app.Container.Singleton(func() file.Storage { return fileStorage })
}

func (p *storageProvider) Boot(app *ioc.Application) error {
	return nil
}

func (p *storageProvider) Terminate() error {
	return nil
}
