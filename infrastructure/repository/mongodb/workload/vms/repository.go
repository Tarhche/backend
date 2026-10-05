// Package vms keeps the workload's VMs in MongoDB.
package vms

import (
	"context"
	"errors"
	"time"

	"github.com/gofrs/uuid/v5"
	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"

	"github.com/khanzadimahdi/testproject/domain"
	"github.com/khanzadimahdi/testproject/domain/workload/vm"
)

const (
	collectionName = "vms"
	queryTimeout   = 3 * time.Second
)

// ErrStale is a copy of a VM that was read before somebody asked the VM for
// something else, written back over what they asked.
//
// Two things write a VM: what is asked of it, by a person or by the control
// plane's own heartbeat, and what its node reports about it. The first moves
// UpdatedAt on and the second never does, so a report that arrives while a
// request is being written is refused rather than allowed to undo it. Whoever
// is refused reads the VM again; a report simply waits for the next beat.
var ErrStale = errors.New("the vm was changed after this copy of it was read")

type VMsRepository struct {
	collection *mongo.Collection
}

var _ vm.Repository = &VMsRepository{}

func NewRepository(database *mongo.Database) *VMsRepository {
	if database == nil {
		panic("database should not be nil")
	}

	return &VMsRepository{
		collection: database.Collection(collectionName),
	}
}

func (r *VMsRepository) GetAll(ctx context.Context, offset uint, limit uint) ([]vm.VM, error) {
	return r.find(ctx, bson.D{}, options.Find().SetSkip(int64(offset)).SetLimit(int64(limit)).SetSort(newestFirst()))
}

func (r *VMsRepository) GetAllByOwner(ctx context.Context, ownerUUID string, offset uint, limit uint) ([]vm.VM, error) {
	return r.find(
		ctx,
		bson.D{{Key: "owner_uuid", Value: ownerUUID}},
		options.Find().SetSkip(int64(offset)).SetLimit(int64(limit)).SetSort(newestFirst()),
	)
}

func (r *VMsRepository) GetAllByOwnerAndKind(ctx context.Context, ownerUUID string, kind vm.Kind) ([]vm.VM, error) {
	return r.find(
		ctx,
		bson.D{{Key: "owner_uuid", Value: ownerUUID}, {Key: "kind", Value: string(kind)}},
		options.Find().SetSort(newestFirst()),
	)
}

func (r *VMsRepository) GetAllByNode(ctx context.Context, nodeName string) ([]vm.VM, error) {
	return r.find(ctx, bson.D{{Key: "node_name", Value: nodeName}}, options.Find().SetSort(newestFirst()))
}

func (r *VMsRepository) CountByOwner(ctx context.Context, ownerUUID string) (uint, error) {
	return r.count(ctx, bson.D{{Key: "owner_uuid", Value: ownerUUID}})
}

func (r *VMsRepository) Count(ctx context.Context) (uint, error) {
	return r.count(ctx, bson.D{})
}

func (r *VMsRepository) GetOne(ctx context.Context, uuid string) (vm.VM, error) {
	return r.findOne(ctx, bson.D{{Key: "_id", Value: uuid}})
}

func (r *VMsRepository) GetOneByOwner(ctx context.Context, ownerUUID string, uuid string) (vm.VM, error) {
	return r.findOne(ctx, bson.D{{Key: "_id", Value: uuid}, {Key: "owner_uuid", Value: ownerUUID}})
}

func (r *VMsRepository) GetOneBySlug(ctx context.Context, slug string) (vm.VM, error) {
	return r.findOne(ctx, bson.D{{Key: "slug", Value: slug}})
}

// Save writes a VM, unless the one stored was asked for something after this
// copy of it was read (ErrStale). A slug another VM already has is
// domain.ErrAlreadyExists, which a new VM answers by taking another one.
func (r *VMsRepository) Save(ctx context.Context, v *vm.VM) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, queryTimeout)
	defer cancel()

	if len(v.UUID) == 0 {
		id, err := uuid.NewV7()
		if err != nil {
			return "", err
		}

		v.UUID = id.String()
	}

	// a VM is saved on every change, so its creation time is whatever it
	// already had rather than the time of the latest write.
	if v.CreatedAt.IsZero() {
		v.CreatedAt = time.Now()
	}

	// the stored VM is replaced only when it has not been asked for anything
	// this copy does not know of. When it has, the filter matches nothing and
	// the upsert tries to insert a second VM under the same uuid, which the
	// database refuses: that refusal is how a stale copy is told apart.
	filter := bson.D{
		{Key: "_id", Value: v.UUID},
		{Key: "updated_at", Value: bson.D{{Key: "$lte", Value: v.UpdatedAt}}},
	}

	_, err := r.collection.UpdateOne(ctx, filter, bson.M{"$set": toBson(v)}, options.UpdateOne().SetUpsert(true))

	return v.UUID, duplicate(err)
}

func (r *VMsRepository) Delete(ctx context.Context, uuid string) error {
	ctx, cancel := context.WithTimeout(ctx, queryTimeout)
	defer cancel()

	_, err := r.collection.DeleteOne(ctx, bson.D{{Key: "_id", Value: uuid}})

	return err
}

func (r *VMsRepository) find(ctx context.Context, filter bson.D, opts ...options.Lister[options.FindOptions]) ([]vm.VM, error) {
	ctx, cancel := context.WithTimeout(ctx, queryTimeout)
	defer cancel()

	cur, err := r.collection.Find(ctx, filter, opts...)
	if err != nil {
		return nil, err
	}
	defer cur.Close(ctx)

	items := make([]vm.VM, 0)
	for cur.Next(ctx) {
		var stored VMBson
		if err := cur.Decode(&stored); err != nil {
			return nil, err
		}

		items = append(items, toVM(&stored))
	}

	if err := cur.Err(); err != nil {
		return nil, err
	}

	return items, nil
}

func (r *VMsRepository) findOne(ctx context.Context, filter bson.D) (vm.VM, error) {
	ctx, cancel := context.WithTimeout(ctx, queryTimeout)
	defer cancel()

	var stored VMBson
	if err := r.collection.FindOne(ctx, filter).Decode(&stored); err != nil {
		if errors.Is(err, mongo.ErrNoDocuments) {
			err = domain.ErrNotExists
		}

		return vm.VM{}, err
	}

	return toVM(&stored), nil
}

func (r *VMsRepository) count(ctx context.Context, filter bson.D) (uint, error) {
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

// duplicate says what a refused write was refused for: the uuid of a VM that
// was changed in the meantime, or a slug another VM already has.
func duplicate(err error) error {
	if err == nil || !mongo.IsDuplicateKeyError(err) {
		return err
	}

	var writeException mongo.WriteException
	if errors.As(err, &writeException) {
		for _, writeError := range writeException.WriteErrors {
			if keyOf(writeError.Raw) == "slug" {
				return domain.ErrAlreadyExists
			}
		}
	}

	return ErrStale
}

// keyOf is the field a duplicate key error names, read from the key value the
// server reports with it.
func keyOf(raw bson.Raw) string {
	keyValue, err := raw.LookupErr("keyValue")
	if err != nil {
		return ""
	}

	document, ok := keyValue.DocumentOK()
	if !ok {
		return ""
	}

	elements, err := document.Elements()
	if err != nil || len(elements) == 0 {
		return ""
	}

	return elements[0].Key()
}
