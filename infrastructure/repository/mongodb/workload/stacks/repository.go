// Package stacks keeps the workload's compose stacks in MongoDB.
package stacks

import (
	"context"
	"errors"
	"time"

	"github.com/gofrs/uuid/v5"
	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"

	"github.com/khanzadimahdi/testproject/domain"
	"github.com/khanzadimahdi/testproject/domain/workload/stack"
)

const (
	collectionName = "stacks"
	queryTimeout   = 3 * time.Second
)

// ErrStale is a copy of a stack that was read before somebody asked it for
// something else, written back over what they asked. What is asked of a stack
// moves UpdatedAt on, and what its node reports about it does not, so a late
// result cannot undo a newer request.
var ErrStale = errors.New("the stack was changed after this copy of it was read")

// StackBson is a stack as it is stored. Nothing that can be cleared is
// omitempty, so that clearing it is written down too.
type StackBson struct {
	UUID          string    `bson:"_id,omitempty"`
	Name          string    `bson:"name"`
	OwnerUUID     string    `bson:"owner_uuid"`
	VMUUID        string    `bson:"vm_uuid"`
	Slug          string    `bson:"slug"`
	Compose       string    `bson:"compose"`
	ExpectedState uint      `bson:"expected_state"`
	State         uint      `bson:"state"`
	Reason        string    `bson:"reason"`
	Output        string    `bson:"output"`
	CreatedAt     time.Time `bson:"created_at"`
	UpdatedAt     time.Time `bson:"updated_at"`
}

type StacksRepository struct {
	collection *mongo.Collection
}

var _ stack.Repository = &StacksRepository{}

func NewRepository(database *mongo.Database) *StacksRepository {
	if database == nil {
		panic("database should not be nil")
	}

	return &StacksRepository{
		collection: database.Collection(collectionName),
	}
}

func (r *StacksRepository) GetAll(ctx context.Context, offset uint, limit uint) ([]stack.Stack, error) {
	return r.find(ctx, bson.D{}, options.Find().SetSkip(int64(offset)).SetLimit(int64(limit)).SetSort(newestFirst()))
}

func (r *StacksRepository) GetAllByOwner(ctx context.Context, ownerUUID string, offset uint, limit uint) ([]stack.Stack, error) {
	return r.find(
		ctx,
		bson.D{{Key: "owner_uuid", Value: ownerUUID}},
		options.Find().SetSkip(int64(offset)).SetLimit(int64(limit)).SetSort(newestFirst()),
	)
}

func (r *StacksRepository) GetAllByVM(ctx context.Context, vmUUID string) ([]stack.Stack, error) {
	return r.find(ctx, bson.D{{Key: "vm_uuid", Value: vmUUID}}, options.Find().SetSort(newestFirst()))
}

func (r *StacksRepository) CountByOwner(ctx context.Context, ownerUUID string) (uint, error) {
	return r.count(ctx, bson.D{{Key: "owner_uuid", Value: ownerUUID}})
}

func (r *StacksRepository) Count(ctx context.Context) (uint, error) {
	return r.count(ctx, bson.D{})
}

func (r *StacksRepository) GetOne(ctx context.Context, uuid string) (stack.Stack, error) {
	return r.findOne(ctx, bson.D{{Key: "_id", Value: uuid}})
}

func (r *StacksRepository) GetOneByOwner(ctx context.Context, ownerUUID string, uuid string) (stack.Stack, error) {
	return r.findOne(ctx, bson.D{{Key: "_id", Value: uuid}, {Key: "owner_uuid", Value: ownerUUID}})
}

func (r *StacksRepository) GetOneBySlug(ctx context.Context, slug string) (stack.Stack, error) {
	return r.findOne(ctx, bson.D{{Key: "slug", Value: slug}})
}

// Save writes a stack, unless the one stored was asked for something after
// this copy of it was read (ErrStale). A slug another stack already has is
// domain.ErrAlreadyExists, which a new stack answers by taking another one.
func (r *StacksRepository) Save(ctx context.Context, s *stack.Stack) (string, error) {
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

	// the stored stack is replaced only when it has not been asked for
	// anything this copy does not know of; otherwise the upsert tries to insert
	// a second stack under the same uuid, which the database refuses.
	filter := bson.D{
		{Key: "_id", Value: s.UUID},
		{Key: "updated_at", Value: bson.D{{Key: "$lte", Value: s.UpdatedAt}}},
	}

	_, err := r.collection.UpdateOne(ctx, filter, bson.M{"$set": toBson(s)}, options.UpdateOne().SetUpsert(true))

	return s.UUID, duplicate(err)
}

func (r *StacksRepository) Delete(ctx context.Context, uuid string) error {
	ctx, cancel := context.WithTimeout(ctx, queryTimeout)
	defer cancel()

	_, err := r.collection.DeleteOne(ctx, bson.D{{Key: "_id", Value: uuid}})

	return err
}

func (r *StacksRepository) find(ctx context.Context, filter bson.D, opts ...options.Lister[options.FindOptions]) ([]stack.Stack, error) {
	ctx, cancel := context.WithTimeout(ctx, queryTimeout)
	defer cancel()

	cur, err := r.collection.Find(ctx, filter, opts...)
	if err != nil {
		return nil, err
	}
	defer cur.Close(ctx)

	items := make([]stack.Stack, 0)
	for cur.Next(ctx) {
		var stored StackBson
		if err := cur.Decode(&stored); err != nil {
			return nil, err
		}

		items = append(items, toStack(&stored))
	}

	if err := cur.Err(); err != nil {
		return nil, err
	}

	return items, nil
}

func (r *StacksRepository) findOne(ctx context.Context, filter bson.D) (stack.Stack, error) {
	ctx, cancel := context.WithTimeout(ctx, queryTimeout)
	defer cancel()

	var stored StackBson
	if err := r.collection.FindOne(ctx, filter).Decode(&stored); err != nil {
		if errors.Is(err, mongo.ErrNoDocuments) {
			err = domain.ErrNotExists
		}

		return stack.Stack{}, err
	}

	return toStack(&stored), nil
}

func (r *StacksRepository) count(ctx context.Context, filter bson.D) (uint, error) {
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

// duplicate says what a refused write was refused for: the uuid of a stack
// that was changed in the meantime, or a slug another stack already has.
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

func toStack(s *StackBson) stack.Stack {
	return stack.Stack{
		UUID:          s.UUID,
		Name:          s.Name,
		OwnerUUID:     s.OwnerUUID,
		VMUUID:        s.VMUUID,
		Slug:          s.Slug,
		Compose:       s.Compose,
		ExpectedState: stack.State(s.ExpectedState),
		State:         stack.State(s.State),
		Reason:        s.Reason,
		Output:        s.Output,
		CreatedAt:     s.CreatedAt,
		UpdatedAt:     s.UpdatedAt,
	}
}

func toBson(s *stack.Stack) StackBson {
	return StackBson{
		UUID:          s.UUID,
		Name:          s.Name,
		OwnerUUID:     s.OwnerUUID,
		VMUUID:        s.VMUUID,
		Slug:          s.Slug,
		Compose:       s.Compose,
		ExpectedState: uint(s.ExpectedState),
		State:         uint(s.State),
		Reason:        s.Reason,
		Output:        s.Output,
		CreatedAt:     s.CreatedAt,
		UpdatedAt:     s.UpdatedAt,
	}
}
