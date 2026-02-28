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
	"github.com/khanzadimahdi/testproject/domain/runner/stats"
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
			Resources: node.Resource{
				Cpu:    t.Resources.Cpu,
				Memory: t.Resources.Memory,
				Disk:   t.Resources.Disk,
			},
			Stats: stats.Stats{
				Memory: stats.Memory{
					Total:     t.Stats.Memory.Total,
					Used:      t.Stats.Memory.Used,
					Available: t.Stats.Memory.Available,
					SwapTotal: t.Stats.Memory.SwapTotal,
					SwapFree:  t.Stats.Memory.SwapFree,
				},
				Disk: stats.Disk{
					Total:      t.Stats.Disk.Total,
					Used:       t.Stats.Disk.Used,
					Available:  t.Stats.Disk.Available,
					FreeInodes: t.Stats.Disk.FreeInodes,
				},
				CPU: stats.CPU{
					ID:        t.Stats.CPU.ID,
					User:      t.Stats.CPU.User,
					Nice:      t.Stats.CPU.Nice,
					System:    t.Stats.CPU.System,
					Idle:      t.Stats.CPU.Idle,
					IOWait:    t.Stats.CPU.IOWait,
					IRQ:       t.Stats.CPU.IRQ,
					SoftIRQ:   t.Stats.CPU.SoftIRQ,
					Steal:     t.Stats.CPU.Steal,
					Guest:     t.Stats.CPU.Guest,
					GuestNice: t.Stats.CPU.GuestNice,
				},
				Load: stats.Load{
					Last1Min:       t.Stats.Load.Last1Min,
					Last5Min:       t.Stats.Load.Last5Min,
					Last15Min:      t.Stats.Load.Last15Min,
					ProcessRunning: t.Stats.Load.ProcessRunning,
					ProcessTotal:   t.Stats.Load.ProcessTotal,
					LastPID:        t.Stats.Load.LastPID,
				},
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
		Resources: node.Resource{
			Cpu:    t.Resources.Cpu,
			Memory: t.Resources.Memory,
			Disk:   t.Resources.Disk,
		},
		Stats: stats.Stats{
			Memory: stats.Memory{
				Total:     t.Stats.Memory.Total,
				Used:      t.Stats.Memory.Used,
				Available: t.Stats.Memory.Available,
				SwapTotal: t.Stats.Memory.SwapTotal,
				SwapFree:  t.Stats.Memory.SwapFree,
			},
			Disk: stats.Disk{
				Total:      t.Stats.Disk.Total,
				Used:       t.Stats.Disk.Used,
				Available:  t.Stats.Disk.Available,
				FreeInodes: t.Stats.Disk.FreeInodes,
			},
			CPU: stats.CPU{
				ID:        t.Stats.CPU.ID,
				User:      t.Stats.CPU.User,
				Nice:      t.Stats.CPU.Nice,
				System:    t.Stats.CPU.System,
				Idle:      t.Stats.CPU.Idle,
				IOWait:    t.Stats.CPU.IOWait,
				IRQ:       t.Stats.CPU.IRQ,
				SoftIRQ:   t.Stats.CPU.SoftIRQ,
				Steal:     t.Stats.CPU.Steal,
				Guest:     t.Stats.CPU.Guest,
				GuestNice: t.Stats.CPU.GuestNice,
			},
			Load: stats.Load{
				Last1Min:       t.Stats.Load.Last1Min,
				Last5Min:       t.Stats.Load.Last5Min,
				Last15Min:      t.Stats.Load.Last15Min,
				ProcessRunning: t.Stats.Load.ProcessRunning,
				ProcessTotal:   t.Stats.Load.ProcessTotal,
				LastPID:        t.Stats.Load.LastPID,
			},
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
		Resources: Resource{
			Cpu:    n.Resources.Cpu,
			Memory: n.Resources.Memory,
			Disk:   n.Resources.Disk,
		},
		Stats: Stats{
			Memory: Memory{
				Total:     n.Stats.Memory.Total,
				Used:      n.Stats.Memory.Used,
				Available: n.Stats.Memory.Available,
				SwapTotal: n.Stats.Memory.SwapTotal,
				SwapFree:  n.Stats.Memory.SwapFree,
			},
			Disk: Disk{
				Total:      n.Stats.Disk.Total,
				Used:       n.Stats.Disk.Used,
				Available:  n.Stats.Disk.Available,
				FreeInodes: n.Stats.Disk.FreeInodes,
			},
			CPU: CPU{
				ID:        n.Stats.CPU.ID,
				User:      n.Stats.CPU.User,
				Nice:      n.Stats.CPU.Nice,
				System:    n.Stats.CPU.System,
				Idle:      n.Stats.CPU.Idle,
				IOWait:    n.Stats.CPU.IOWait,
				IRQ:       n.Stats.CPU.IRQ,
				SoftIRQ:   n.Stats.CPU.SoftIRQ,
				Steal:     n.Stats.CPU.Steal,
				Guest:     n.Stats.CPU.Guest,
				GuestNice: n.Stats.CPU.GuestNice,
			},
			Load: Load{
				Last1Min:       n.Stats.Load.Last1Min,
				Last5Min:       n.Stats.Load.Last5Min,
				Last15Min:      n.Stats.Load.Last15Min,
				ProcessRunning: n.Stats.Load.ProcessRunning,
				ProcessTotal:   n.Stats.Load.ProcessTotal,
				LastPID:        n.Stats.Load.LastPID,
			},
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
