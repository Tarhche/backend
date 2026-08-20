package health

import (
	"context"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"

	"github.com/khanzadimahdi/testproject/domain"
)

// MongodbPinger checks that the database answers. it runs the same `ping`
// command the mongodb provider runs at startup, so it covers reachability,
// authentication and the database actually being selectable, not just a live
// TCP connection.
type MongodbPinger struct {
	database *mongo.Database
}

var _ domain.Pinger = &MongodbPinger{}

func NewMongodbPinger(database *mongo.Database) *MongodbPinger {
	return &MongodbPinger{
		database: database,
	}
}

func (p *MongodbPinger) Ping(ctx context.Context) error {
	// without a deadline of its own the driver spends its full server selection
	// timeout (30s by default) looking for a reachable server, long after the
	// container healthcheck has given up waiting for an answer
	ctx, cancel := context.WithTimeout(ctx, pingTimeout)
	defer cancel()

	return p.database.RunCommand(ctx, bson.D{{Key: "ping", Value: 1}}).Err()
}
