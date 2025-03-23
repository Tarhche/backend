package providers

import (
	"context"
	"fmt"
	"os"

	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"

	"github.com/khanzadimahdi/testproject/infrastructure/ioc"
)

type mongodbProvider struct {
	terminate func()
}

var _ ioc.ServiceProvider = &mongodbProvider{}

func NewMongodbProvider() *mongodbProvider {
	return &mongodbProvider{}
}

func (p *mongodbProvider) Register(ctx context.Context, iocContainer ioc.ServiceContainer) error {
	uri := fmt.Sprintf(
		"%s://%s:%s@%s:%s",
		os.Getenv("MONGO_SCHEME"),
		os.Getenv("MONGO_USERNAME"),
		os.Getenv("MONGO_PASSWORD"),
		os.Getenv("MONGO_HOST"),
		os.Getenv("MONGO_PORT"),
	)

	mongoClient, err := mongo.Connect(ctx, options.Client().ApplyURI(uri))
	if err != nil {
		return err
	}
	database := mongoClient.Database(os.Getenv("MONGO_DATABASE_NAME"))

	p.terminate = func() {
		mongoClient.Disconnect(context.Background())
	}

	return iocContainer.Singleton(func() *mongo.Database { return database })
}

func (p *mongodbProvider) Boot(ctx context.Context, iocContainer ioc.ServiceContainer) error {
	return nil
}

func (p *mongodbProvider) Terminate() error {
	if p.terminate != nil {
		p.terminate()
	}

	return nil
}
