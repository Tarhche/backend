package providers

import (
	"context"

	"github.com/danceable/provider"
	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"

	"github.com/khanzadimahdi/testproject/infrastructure/configs"
	tracing "github.com/khanzadimahdi/testproject/infrastructure/telemetry/trace"
)

type mongodbProvider struct {
	terminate func()
}

var _ provider.Provider = &mongodbProvider{}

func NewMongodbProvider() *mongodbProvider {
	return &mongodbProvider{}
}

func (p *mongodbProvider) Register(ctx context.Context, c provider.Container) error {
	var globalConfigs *configs.Global
	if err := c.Resolve(&globalConfigs); err != nil {
		return err
	}

	uri := globalConfigs.Mongo.URI()

	serverAPIVersion := options.ServerAPI(options.ServerAPIVersion1)
	connectionOptions := options.Client().
		ApplyURI(uri).
		SetServerAPIOptions(serverAPIVersion).
		SetMonitor(tracing.NewMongoCommandMonitor("mongodb"))

	mongoClient, err := mongo.Connect(connectionOptions)
	if err != nil {
		return err
	}

	if err := mongoClient.Ping(ctx, nil); err != nil {
		return err
	}

	database := mongoClient.Database(globalConfigs.Mongo.DatabaseName)

	var result bson.M
	if err := database.RunCommand(ctx, bson.D{{Key: "ping", Value: 1}}).Decode(&result); err != nil {
		return err
	}

	p.terminate = func() {
		mongoClient.Disconnect(context.Background())
	}

	return c.Bind(func() *mongo.Database { return database }, provider.Singleton())
}

func (p *mongodbProvider) Boot(ctx context.Context, c provider.Container) error {
	return nil
}

func (p *mongodbProvider) Terminate(ctx context.Context) error {
	if p.terminate != nil {
		p.terminate()
	}

	return nil
}
