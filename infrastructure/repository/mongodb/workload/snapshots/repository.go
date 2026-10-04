// Package snapshots keeps the records of the workload's VM snapshots in
// MongoDB. The archives themselves are in the snapshots bucket.
package snapshots

import (
	"context"
	"errors"
	"time"

	"github.com/gofrs/uuid/v5"
	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"

	"github.com/khanzadimahdi/testproject/domain"
	"github.com/khanzadimahdi/testproject/domain/workload/snapshot"
	"github.com/khanzadimahdi/testproject/domain/workload/vm"
)

const (
	collectionName = "snapshots"
	queryTimeout   = 3 * time.Second
)

// SnapshotBson is a snapshot as it is stored. Nothing that can be cleared is
// omitempty, so that clearing it is written down too.
type SnapshotBson struct {
	UUID        string    `bson:"_id,omitempty"`
	Name        string    `bson:"name"`
	OwnerUUID   string    `bson:"owner_uuid"`
	VMUUID      string    `bson:"vm_uuid"`
	VMName      string    `bson:"vm_name"`
	Kind        string    `bson:"kind"`
	Image       string    `bson:"image"`
	Disk        uint64    `bson:"disk"`
	Engine      string    `bson:"engine"`
	Size        int64     `bson:"size"`
	State       uint      `bson:"state"`
	Reason      string    `bson:"reason"`
	CreatedAt   time.Time `bson:"created_at"`
	CompletedAt time.Time `bson:"completed_at"`
}

type SnapshotsRepository struct {
	collection *mongo.Collection
}

var _ snapshot.Repository = &SnapshotsRepository{}

func NewRepository(database *mongo.Database) *SnapshotsRepository {
	if database == nil {
		panic("database should not be nil")
	}

	return &SnapshotsRepository{
		collection: database.Collection(collectionName),
	}
}

func (r *SnapshotsRepository) GetAll(ctx context.Context, offset uint, limit uint) ([]snapshot.Snapshot, error) {
	return r.find(ctx, bson.D{}, options.Find().SetSkip(int64(offset)).SetLimit(int64(limit)).SetSort(newestFirst()))
}

func (r *SnapshotsRepository) GetAllByOwner(ctx context.Context, ownerUUID string, offset uint, limit uint) ([]snapshot.Snapshot, error) {
	return r.find(
		ctx,
		bson.D{{Key: "owner_uuid", Value: ownerUUID}},
		options.Find().SetSkip(int64(offset)).SetLimit(int64(limit)).SetSort(newestFirst()),
	)
}

func (r *SnapshotsRepository) GetAllByVM(ctx context.Context, vmUUID string) ([]snapshot.Snapshot, error) {
	return r.find(ctx, bson.D{{Key: "vm_uuid", Value: vmUUID}}, options.Find().SetSort(newestFirst()))
}

func (r *SnapshotsRepository) CountByOwner(ctx context.Context, ownerUUID string) (uint, error) {
	return r.count(ctx, bson.D{{Key: "owner_uuid", Value: ownerUUID}})
}

func (r *SnapshotsRepository) Count(ctx context.Context) (uint, error) {
	return r.count(ctx, bson.D{})
}

func (r *SnapshotsRepository) GetOne(ctx context.Context, uuid string) (snapshot.Snapshot, error) {
	return r.findOne(ctx, bson.D{{Key: "_id", Value: uuid}})
}

func (r *SnapshotsRepository) GetOneByOwner(ctx context.Context, ownerUUID string, uuid string) (snapshot.Snapshot, error) {
	return r.findOne(ctx, bson.D{{Key: "_id", Value: uuid}, {Key: "owner_uuid", Value: ownerUUID}})
}

func (r *SnapshotsRepository) Save(ctx context.Context, s *snapshot.Snapshot) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, queryTimeout)
	defer cancel()

	if len(s.UUID) == 0 {
		id, err := uuid.NewV7()
		if err != nil {
			return "", err
		}

		s.UUID = id.String()
	}

	if s.CreatedAt.IsZero() {
		s.CreatedAt = time.Now()
	}

	if _, err := r.collection.UpdateOne(
		ctx,
		bson.D{{Key: "_id", Value: s.UUID}},
		bson.M{"$set": toBson(s)},
		options.UpdateOne().SetUpsert(true),
	); err != nil {
		return "", err
	}

	return s.UUID, nil
}

func (r *SnapshotsRepository) Delete(ctx context.Context, uuid string) error {
	ctx, cancel := context.WithTimeout(ctx, queryTimeout)
	defer cancel()

	_, err := r.collection.DeleteOne(ctx, bson.D{{Key: "_id", Value: uuid}})

	return err
}

func (r *SnapshotsRepository) find(ctx context.Context, filter bson.D, opts ...options.Lister[options.FindOptions]) ([]snapshot.Snapshot, error) {
	ctx, cancel := context.WithTimeout(ctx, queryTimeout)
	defer cancel()

	cur, err := r.collection.Find(ctx, filter, opts...)
	if err != nil {
		return nil, err
	}
	defer cur.Close(ctx)

	items := make([]snapshot.Snapshot, 0)
	for cur.Next(ctx) {
		var stored SnapshotBson
		if err := cur.Decode(&stored); err != nil {
			return nil, err
		}

		items = append(items, toSnapshot(&stored))
	}

	if err := cur.Err(); err != nil {
		return nil, err
	}

	return items, nil
}

func (r *SnapshotsRepository) findOne(ctx context.Context, filter bson.D) (snapshot.Snapshot, error) {
	ctx, cancel := context.WithTimeout(ctx, queryTimeout)
	defer cancel()

	var stored SnapshotBson
	if err := r.collection.FindOne(ctx, filter).Decode(&stored); err != nil {
		if errors.Is(err, mongo.ErrNoDocuments) {
			err = domain.ErrNotExists
		}

		return snapshot.Snapshot{}, err
	}

	return toSnapshot(&stored), nil
}

func (r *SnapshotsRepository) count(ctx context.Context, filter bson.D) (uint, error) {
	ctx, cancel := context.WithTimeout(ctx, queryTimeout)
	defer cancel()

	c, err := r.collection.CountDocuments(ctx, filter)
	if err != nil {
		return 0, err
	}

	return uint(c), nil
}

// newestFirst orders by uuid, which is a v7 and so orders by creation.
func newestFirst() bson.D {
	return bson.D{{Key: "_id", Value: -1}}
}

func toSnapshot(s *SnapshotBson) snapshot.Snapshot {
	return snapshot.Snapshot{
		UUID:        s.UUID,
		Name:        s.Name,
		OwnerUUID:   s.OwnerUUID,
		VMUUID:      s.VMUUID,
		VMName:      s.VMName,
		Kind:        vm.Kind(s.Kind),
		Image:       s.Image,
		Disk:        s.Disk,
		Engine:      s.Engine,
		Size:        s.Size,
		State:       snapshot.State(s.State),
		Reason:      s.Reason,
		CreatedAt:   s.CreatedAt,
		CompletedAt: s.CompletedAt,
	}
}

func toBson(s *snapshot.Snapshot) SnapshotBson {
	return SnapshotBson{
		UUID:        s.UUID,
		Name:        s.Name,
		OwnerUUID:   s.OwnerUUID,
		VMUUID:      s.VMUUID,
		VMName:      s.VMName,
		Kind:        string(s.Kind),
		Image:       s.Image,
		Disk:        s.Disk,
		Engine:      s.Engine,
		Size:        s.Size,
		State:       uint(s.State),
		Reason:      s.Reason,
		CreatedAt:   s.CreatedAt,
		CompletedAt: s.CompletedAt,
	}
}
