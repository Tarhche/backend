package nodes

import (
	"context"
	"errors"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"

	"github.com/khanzadimahdi/testproject/domain"
	"github.com/khanzadimahdi/testproject/domain/runner/node"
)

const (
	collectionName = "nodes"
	queryTimeout   = 3 * time.Second
)

type NodesRepository struct {
	collection *mongo.Collection
}

var _ node.Repository = &NodesRepository{}

func NewRepository(database *mongo.Database) *NodesRepository {
	if database == nil {
		panic("database should not be nil")
	}

	return &NodesRepository{
		collection: database.Collection(collectionName),
	}
}

func (r *NodesRepository) GetAll(offset uint, limit uint) ([]node.Node, error) {
	ctx, cancel := context.WithTimeout(context.Background(), queryTimeout)
	defer cancel()

	o := int64(offset)
	l := int64(limit)
	desc := bson.D{{Key: "_id", Value: -1}}

	cur, err := r.collection.Find(ctx, bson.D{}, options.Find().SetSkip(o).SetLimit(l).SetSort(desc))
	if err != nil {
		return nil, err
	}
	defer cur.Close(ctx)

	items := make([]node.Node, 0, limit)
	for cur.Next(ctx) {
		var t NodeBson

		if err := cur.Decode(&t); err != nil {
			return nil, err
		}
		items = append(items, node.Node{
			Name: t.Name,
			Role: node.Role(t.Role),
			Stats: node.Stats{
				PIDs:          t.Stats.PIDs,
				CPUPercent:    t.Stats.CPUPercent,
				MemoryUsage:   t.Stats.MemoryUsage,
				MemoryLimit:   t.Stats.MemoryLimit,
				MemoryPercent: t.Stats.MemoryPercent,
				NetworkInput:  t.Stats.NetworkInput,
				NetworkOutput: t.Stats.NetworkOutput,
				BlockInput:    t.Stats.BlockInput,
				BlockOutput:   t.Stats.BlockOutput,
			},
			LastHeartbeatAt: t.LastHeartbeatAt,
		})
	}

	if err := cur.Err(); err != nil {
		return nil, err
	}

	return items, nil
}

func (r *NodesRepository) GetOne(UUID string) (node.Node, error) {
	ctx, cancel := context.WithTimeout(context.Background(), queryTimeout)
	defer cancel()

	filter := bson.D{{Key: "_id", Value: UUID}}

	var t NodeBson
	if err := r.collection.FindOne(ctx, filter).Decode(&t); err != nil {
		if errors.Is(err, mongo.ErrNoDocuments) {
			err = domain.ErrNotExists
		}
		return node.Node{}, err
	}

	return node.Node{
		Name: t.Name,
		Role: node.Role(t.Role),
		Stats: node.Stats{
			PIDs:          t.Stats.PIDs,
			CPUPercent:    t.Stats.CPUPercent,
			MemoryUsage:   t.Stats.MemoryUsage,
			MemoryLimit:   t.Stats.MemoryLimit,
			MemoryPercent: t.Stats.MemoryPercent,
			NetworkInput:  t.Stats.NetworkInput,
			NetworkOutput: t.Stats.NetworkOutput,
			BlockInput:    t.Stats.BlockInput,
			BlockOutput:   t.Stats.BlockOutput,
		},
		LastHeartbeatAt: t.LastHeartbeatAt,
	}, nil
}

func (r *NodesRepository) Save(n *node.Node) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), queryTimeout)
	defer cancel()

	update := NodeBson{
		Name: n.Name,
		Role: string(n.Role),
		Stats: Stats{
			PIDs:          n.Stats.PIDs,
			CPUPercent:    n.Stats.CPUPercent,
			MemoryUsage:   n.Stats.MemoryUsage,
			MemoryLimit:   n.Stats.MemoryLimit,
			MemoryPercent: n.Stats.MemoryPercent,
			NetworkInput:  n.Stats.NetworkInput,
			NetworkOutput: n.Stats.NetworkOutput,
			BlockInput:    n.Stats.BlockInput,
			BlockOutput:   n.Stats.BlockOutput,
		},
		LastHeartbeatAt: n.LastHeartbeatAt,
		CreatedAt:       time.Now(),
		UpdatedAt:       time.Now(),
	}

	if _, err := r.collection.UpdateOne(
		ctx,
		bson.D{{Key: "_id", Value: n.Name}},
		bson.M{"$set": update},
		options.UpdateOne().SetUpsert(true),
	); err != nil {
		return "", err
	}

	return n.Name, nil
}

func (r *NodesRepository) Delete(UUID string) error {
	ctx, cancel := context.WithTimeout(context.Background(), queryTimeout)
	defer cancel()

	_, err := r.collection.DeleteOne(ctx, bson.D{{Key: "_id", Value: UUID}})

	return err
}

func (r *NodesRepository) Count() (uint, error) {
	ctx, cancel := context.WithTimeout(context.Background(), queryTimeout)
	defer cancel()

	c, err := r.collection.CountDocuments(ctx, bson.D{})
	if err != nil {
		return uint(c), err
	}

	return uint(c), nil
}
