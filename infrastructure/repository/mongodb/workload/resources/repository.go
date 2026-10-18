// Package resources keeps the resources of every kind in MongoDB.
//
// Every kind's are kept in one collection, workloads, and everything the
// repository reads, writes or deletes is of one kind, which every query it
// runs names: what is asked of one kind never reaches another's. A document
// is the resource's manifest, its kind, metadata, spec and status, beside its
// version and what the control plane keeps of what it is in the middle of
// asking:
//
//	{_id: <uuid>, kind, metadata: {name, slug, owner_uuid, labels, owners,
//	 refs, node, lifetime, expires_at, generation, deletion_requested_at,
//	 created_at, updated_at}, spec: {…}, status: {…}, version,
//	 control: {pending, attempts, tried_at, answer, reset, reset_at,
//	 restores}}
//
// Its uuid is its _id, so no two resources share one, whatever their kinds,
// and no two share a slug either: a VM's and a task's are what the ingress
// serves their ports under and a stack's is what its graph names what it
// makes under, and the one collection holds each to one resource.
//
// The spec and the status are the kind's own JSON, kept as documents of
// their own so that they can be read in the database as they are in the API.
// They are kept as they are written, whatever their keys say: a label whose
// key is one of extended JSON's own, such as $date or $numberLong, which a
// person or an image may give, is a label like any other, kept under a field
// of that name, as the database keeps any name beneath a document's own
// fields, a dot or a dollar in it. Each number is kept as relaxed extended
// JSON reads one, and they are read back as relaxed extended JSON, which is
// the JSON they were written as, but for an integer beyond what 64 bits hold,
// kept and read back as a double. One kept while they were read from JSON as
// relaxed extended JSON, which read an object of one of its own keys as what
// that key says, a date say, is read back as it was kept. What the database
// cannot keep as it is, a key with a NUL in it, or a document beyond what one
// holds, is resource.ErrUnstorable.
//
// # Indexes
//
// The collection is indexed the ways resources are read:
//
//   - by slug, unique among the resources that have one, whatever their
//     kinds: a partial index, of the documents with a slug, rather than a
//     sparse one, whose documents left out change once it is compound;
//   - every resource of a kind, newest first, which the reconcile loop pages
//     through and a listing narrowed by nothing reads, as the reconciler's
//     resync does, narrowed to what nothing owns;
//   - a person's own of a kind, newest first;
//   - what a node holds of a kind;
//   - and what belongs to a resource, of any kind, by its refs and by its
//     owners, which is how what lives in a resource is found, a Docker VM's
//     building blocks among it, and what uses or owns one.
//
// What lives in a resource names it among its refs, and what a resource owns
// names it among its owners: a listing of what belongs to one looks in both.
//
// A resource read by its uuid or its slug needs nothing led by its kind: each
// names one document at most.
//
// It is indexed by the control plane as it starts, for every kind it
// registers (EnsureKind), rather than by a migration. The migrations are a
// fixed list, applied once each by `app migrate`, which runs without the
// control plane's registry and so cannot know which kinds there are; and a
// released migration is never edited, so none could be told of an index
// added later. Creating an index that is already there, with the same keys
// and options, changes nothing, so doing it at every start costs one round
// trip per kind, as the task logs' index already does.
//
// No migration moves what was kept before workloads into it: that is moved
// by hand. A control plane started on an empty workloads while the nodes
// hold VMs takes each VM a node reports for one nobody keeps a record of, and
// asks its node to delete it.
package resources

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"maps"
	"math"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/gofrs/uuid/v5"
	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
	"go.mongodb.org/mongo-driver/v2/x/mongo/driver"

	"github.com/khanzadimahdi/testproject/domain"
	"github.com/khanzadimahdi/testproject/domain/workload/kind"
	"github.com/khanzadimahdi/testproject/domain/workload/resource"
)

const (
	queryTimeout = 3 * time.Second

	// collectionName is the collection every kind's resources are kept in.
	collectionName = "workloads"

	// codeTooLarge and codeTooDeep are what the database refuses a document
	// with that is beyond what a document holds (BSONObjectTooLarge), and
	// nested deeper than it nests one (Overflow).
	codeTooLarge = 10334
	codeTooDeep  = 15
)

// Repository keeps the resources of the kinds it was told of.
type Repository struct {
	workloads *mongo.Collection

	lock  sync.RWMutex
	kinds map[string]bool
}

var _ resource.Repository = &Repository{}

func NewRepository(database *mongo.Database) *Repository {
	if database == nil {
		panic("database should not be nil")
	}

	return &Repository{workloads: database.Collection(collectionName), kinds: make(map[string]bool)}
}

// EnsureKind has the repository keep a kind's resources, in workloads beside
// every other kind's, and makes sure workloads is indexed the ways resources
// are read, which it may be already. A kind it was not told of is not kept:
// asked for one, every method says kind.ErrUnknownKind.
func (r *Repository) EnsureKind(ctx context.Context, d kind.Descriptor) error {
	if len(d.Name) == 0 {
		return errors.New("a kind is kept under its name, and this one has none")
	}

	ctx, cancel := context.WithTimeout(ctx, queryTimeout)
	defer cancel()

	if _, err := r.workloads.Indexes().CreateMany(ctx, []mongo.IndexModel{
		{Keys: bson.D{{Key: "metadata.slug", Value: 1}}, Options: options.Index().SetUnique(true).SetPartialFilterExpression(bson.D{
			{Key: "metadata.slug", Value: bson.D{{Key: "$exists", Value: true}}},
		})},
		{Keys: bson.D{{Key: "kind", Value: 1}, {Key: "_id", Value: -1}}},
		{Keys: bson.D{{Key: "kind", Value: 1}, {Key: "metadata.owner_uuid", Value: 1}, {Key: "_id", Value: -1}}},
		{Keys: bson.D{{Key: "kind", Value: 1}, {Key: "metadata.node", Value: 1}}},
		{Keys: bson.D{{Key: "metadata.owners.uuid", Value: 1}, {Key: "metadata.owners.kind", Value: 1}}},
		{Keys: bson.D{{Key: "metadata.refs.uuid", Value: 1}, {Key: "metadata.refs.kind", Value: 1}}},
	}); err != nil {
		return fmt.Errorf("%s cannot be indexed: %w", collectionName, err)
	}

	r.lock.Lock()
	defer r.lock.Unlock()

	r.kinds[d.Name] = true

	return nil
}

func (r *Repository) Create(ctx context.Context, record resource.Record) (resource.Record, error) {
	collection, err := r.collection(record.Kind)
	if err != nil {
		return resource.Record{}, err
	}

	if len(record.Metadata.UUID) == 0 {
		id, err := uuid.NewV7()
		if err != nil {
			return resource.Record{}, err
		}

		record.Metadata.UUID = id.String()
	}

	record.Version = 1

	stored, err := toDocument(record)
	if err != nil {
		return resource.Record{}, err
	}

	ctx, cancel := context.WithTimeout(ctx, queryTimeout)
	defer cancel()

	if _, err := collection.InsertOne(ctx, stored); err != nil {
		if mongo.IsDuplicateKeyError(err) {
			return resource.Record{}, domain.ErrAlreadyExists
		}

		return resource.Record{}, unstorable(err)
	}

	return toRecord(stored)
}

func (r *Repository) Update(ctx context.Context, record resource.Record) (resource.Record, error) {
	collection, err := r.collection(record.Kind)
	if err != nil {
		return resource.Record{}, err
	}

	read := record.Version
	record.Version++

	stored, err := toDocument(record)
	if err != nil {
		return resource.Record{}, err
	}

	ctx, cancel := context.WithTimeout(ctx, queryTimeout)
	defer cancel()

	// only the version this copy was read at is replaced: a resource written
	// since is at another, and matches nothing; nor does one of another kind.
	ofItsKind := bson.D{{Key: "_id", Value: stored.UUID}, {Key: "kind", Value: stored.Kind}}

	replaced, err := collection.ReplaceOne(ctx, slices.Concat(ofItsKind, bson.D{{Key: "version", Value: read}}), stored)
	if mongo.IsDuplicateKeyError(err) {
		return resource.Record{}, domain.ErrAlreadyExists
	} else if err != nil {
		return resource.Record{}, unstorable(err)
	}

	if replaced.MatchedCount == 0 {
		exists, err := collection.CountDocuments(ctx, ofItsKind)
		if err != nil {
			return resource.Record{}, err
		}

		if exists == 0 {
			return resource.Record{}, domain.ErrNotExists
		}

		return resource.Record{}, resource.ErrConflict
	}

	return toRecord(stored)
}

func (r *Repository) Delete(ctx context.Context, kindName string, uuid string) error {
	collection, err := r.collection(kindName)
	if err != nil {
		return err
	}

	ctx, cancel := context.WithTimeout(ctx, queryTimeout)
	defer cancel()

	_, err = collection.DeleteOne(ctx, bson.D{{Key: "_id", Value: uuid}, {Key: "kind", Value: kindName}})

	return err
}

func (r *Repository) DeleteAt(ctx context.Context, kindName string, uuid string, version int64) error {
	collection, err := r.collection(kindName)
	if err != nil {
		return err
	}

	ctx, cancel := context.WithTimeout(ctx, queryTimeout)
	defer cancel()

	// only the version it was read at is taken away: one written since is at
	// another, and matches nothing; nor does one of another kind.
	ofItsKind := bson.D{{Key: "_id", Value: uuid}, {Key: "kind", Value: kindName}}

	deleted, err := collection.DeleteOne(ctx, slices.Concat(ofItsKind, bson.D{{Key: "version", Value: version}}))
	if err != nil {
		return err
	}

	if deleted.DeletedCount == 0 {
		exists, err := collection.CountDocuments(ctx, ofItsKind)
		if err != nil {
			return err
		}

		if exists == 0 {
			return domain.ErrNotExists
		}

		return resource.ErrConflict
	}

	return nil
}

func (r *Repository) GetOne(ctx context.Context, kindName string, uuid string) (resource.Record, error) {
	return r.findOne(ctx, kindName, bson.D{{Key: "_id", Value: uuid}})
}

func (r *Repository) GetOneByOwner(ctx context.Context, kindName string, ownerUUID string, uuid string) (resource.Record, error) {
	return r.findOne(ctx, kindName, bson.D{{Key: "_id", Value: uuid}, {Key: "metadata.owner_uuid", Value: ownerUUID}})
}

func (r *Repository) GetOneBySlug(ctx context.Context, kindName string, slug string) (resource.Record, error) {
	if len(slug) == 0 {
		return resource.Record{}, domain.ErrNotExists
	}

	return r.findOne(ctx, kindName, bson.D{{Key: "metadata.slug", Value: slug}})
}

func (r *Repository) GetAll(ctx context.Context, kindName string, filter resource.Filter, offset uint, limit uint) ([]resource.Record, uint, error) {
	collection, err := r.collection(kindName)
	if err != nil {
		return nil, 0, err
	}

	ctx, cancel := context.WithTimeout(ctx, queryTimeout)
	defer cancel()

	query := filterOf(kindName, filter)

	total, err := collection.CountDocuments(ctx, query)
	if err != nil {
		return nil, 0, err
	}

	// newest first, by uuid, as a v7 orders by when it was made.
	find := options.Find().SetSort(bson.D{{Key: "_id", Value: -1}}).SetSkip(int64(offset))
	if limit > 0 {
		find.SetLimit(int64(limit))
	}

	cursor, err := collection.Find(ctx, query, find)
	if err != nil {
		return nil, 0, err
	}
	defer cursor.Close(ctx)

	records := make([]resource.Record, 0)
	for cursor.Next(ctx) {
		var stored document
		if err := cursor.Decode(&stored); err != nil {
			return nil, 0, err
		}

		record, err := toRecord(stored)
		if err != nil {
			return nil, 0, err
		}

		records = append(records, record)
	}

	if err := cursor.Err(); err != nil {
		return nil, 0, err
	}

	return records, uint(total), nil
}

// findOne is the one resource of the kind that filter lets through.
func (r *Repository) findOne(ctx context.Context, kindName string, filter bson.D) (resource.Record, error) {
	collection, err := r.collection(kindName)
	if err != nil {
		return resource.Record{}, err
	}

	ctx, cancel := context.WithTimeout(ctx, queryTimeout)
	defer cancel()

	var stored document
	if err := collection.FindOne(ctx, append(bson.D{{Key: "kind", Value: kindName}}, filter...)).Decode(&stored); err != nil {
		if errors.Is(err, mongo.ErrNoDocuments) {
			return resource.Record{}, domain.ErrNotExists
		}

		return resource.Record{}, err
	}

	return toRecord(stored)
}

// collection is the collection a kind's resources are kept in, which is
// every kind's, once the repository was told of the kind.
func (r *Repository) collection(kindName string) (*mongo.Collection, error) {
	r.lock.RLock()
	defer r.lock.RUnlock()

	if !r.kinds[kindName] {
		return nil, fmt.Errorf("%w: no %q is kept here", kind.ErrUnknownKind, kindName)
	}

	return r.workloads, nil
}

// filterOf is what a filter lets through of a kind's resources, as a query.
func filterOf(kindName string, filter resource.Filter) bson.D {
	query := bson.D{{Key: "kind", Value: kindName}}

	if len(filter.OwnerUUID) > 0 {
		query = append(query, bson.E{Key: "metadata.owner_uuid", Value: filter.OwnerUUID})
	}

	if len(filter.Node) > 0 {
		query = append(query, bson.E{Key: "metadata.node", Value: filter.Node})
	}

	// what belongs to a resource names it among its refs, or among its
	// owners.
	switch parent := filter.Parent; {
	case len(parent.UUID) == 0:
	case len(parent.Kind) == 0:
		query = append(query, bson.E{Key: "$or", Value: bson.A{
			bson.D{{Key: "metadata.refs.uuid", Value: parent.UUID}},
			bson.D{{Key: "metadata.owners.uuid", Value: parent.UUID}},
		}})
	default:
		named := bson.D{{Key: "$elemMatch", Value: bson.D{
			{Key: "kind", Value: parent.Kind},
			{Key: "uuid", Value: parent.UUID},
		}}}

		query = append(query, bson.E{Key: "$or", Value: bson.A{
			bson.D{{Key: "metadata.refs", Value: named}},
			bson.D{{Key: "metadata.owners", Value: named}},
		}})
	}

	// what nothing owns names no owner.
	if filter.Unowned {
		query = append(query, bson.E{Key: "metadata.owners.0", Value: bson.D{{Key: "$exists", Value: false}}})
	}

	// a label's key has dots in it, workload.managed-by, which a path would
	// take for fields within fields, so each is read as the one field it is.
	if len(filter.Labels) > 0 {
		carried := make(bson.A, 0, len(filter.Labels))

		for _, key := range slices.Sorted(maps.Keys(filter.Labels)) {
			carried = append(carried, bson.D{{Key: "$eq", Value: bson.A{
				bson.D{{Key: "$getField", Value: bson.D{
					{Key: "field", Value: bson.D{{Key: "$literal", Value: key}}},
					{Key: "input", Value: "$metadata.labels"},
				}}},
				filter.Labels[key],
			}}})
		}

		query = append(query, bson.E{Key: "$expr", Value: bson.D{{Key: "$and", Value: carried}}})
	}

	return query
}

// document is a resource as it is stored. Nothing is omitted that a resource
// may have had and lost, but by leaving it out: a document is replaced whole.
type document struct {
	UUID     string        `bson:"_id"`
	Kind     string        `bson:"kind"`
	Metadata metadata      `bson:"metadata"`
	Spec     bson.RawValue `bson:"spec"`
	Status   bson.RawValue `bson:"status"`
	Version  int64         `bson:"version"`
	Control  control       `bson:"control"`
}

// metadata leaves out a slug it does not have, so the unique index on slugs,
// which is of the documents that have one, holds only the resources reached
// under a name.
type metadata struct {
	Name                string            `bson:"name,omitempty"`
	Slug                string            `bson:"slug,omitempty"`
	OwnerUUID           string            `bson:"owner_uuid,omitempty"`
	Labels              map[string]string `bson:"labels,omitempty"`
	Owners              []reference       `bson:"owners,omitempty"`
	Refs                []reference       `bson:"refs,omitempty"`
	Node                string            `bson:"node,omitempty"`
	Lifetime            int64             `bson:"lifetime,omitempty"`
	ExpiresAt           *time.Time        `bson:"expires_at,omitempty"`
	Generation          int64             `bson:"generation,omitempty"`
	DeletionRequestedAt *time.Time        `bson:"deletion_requested_at,omitempty"`
	CreatedAt           time.Time         `bson:"created_at"`
	UpdatedAt           time.Time         `bson:"updated_at"`
}

type reference struct {
	Kind string `bson:"kind"`
	UUID string `bson:"uuid"`
}

// control is what the control plane keeps of what it is asking a resource.
type control struct {
	Pending  *pending   `bson:"pending,omitempty"`
	Attempts int        `bson:"attempts,omitempty"`
	TriedAt  *time.Time `bson:"tried_at,omitempty"`
	Answer   *answer    `bson:"answer,omitempty"`
	Reset    bool       `bson:"reset,omitempty"`
	ResetAt  *time.Time `bson:"reset_at,omitempty"`
	Restores []string   `bson:"restores,omitempty"`
}

type pending struct {
	Action  string        `bson:"action"`
	Payload bson.RawValue `bson:"payload"`
	IDs     []string      `bson:"ids"`
	SentAt  time.Time     `bson:"sent_at"`
}

type answer struct {
	ID      string    `bson:"id"`
	Kind    string    `bson:"kind"`
	UUID    string    `bson:"uuid"`
	Action  string    `bson:"action"`
	Node    string    `bson:"node,omitempty"`
	Attempt int       `bson:"attempt,omitempty"`
	OK      bool      `bson:"ok"`
	Refused bool      `bson:"refused,omitempty"`
	Reason  string    `bson:"reason,omitempty"`
	Output  string    `bson:"output,omitempty"`
	At      time.Time `bson:"at"`
}

// unstorable is err, what writing a document failed with, said to be the
// document's own when it is (resource.ErrUnstorable): one the driver cannot
// write, a key with a NUL in it say, or one beyond what a message holds; or
// one the database refuses as beyond what a document holds, or as nested
// deeper than it nests one.
func unstorable(err error) error {
	var (
		written mongo.MarshalError
		refused mongo.ServerError
	)

	switch {
	case errors.As(err, &written), errors.Is(err, driver.ErrDocumentTooLarge):
	case errors.As(err, &refused) && (refused.HasErrorCode(codeTooLarge) || refused.HasErrorCode(codeTooDeep)):
	default:
		return err
	}

	return fmt.Errorf("%w: %w", resource.ErrUnstorable, err)
}

func toDocument(record resource.Record) (document, error) {
	spec, err := valueOf(record.Spec)
	if err != nil {
		return document{}, fmt.Errorf("the %s's spec cannot be stored: %w", record.Kind, err)
	}

	status, err := valueOf(record.Status)
	if err != nil {
		return document{}, fmt.Errorf("the %s's status cannot be stored: %w", record.Kind, err)
	}

	m := record.Metadata

	stored := document{
		UUID: m.UUID,
		Kind: record.Kind,
		Metadata: metadata{
			Name:                m.Name,
			Slug:                m.Slug,
			OwnerUUID:           m.OwnerUUID,
			Labels:              maps.Clone(m.Labels),
			Node:                m.Node,
			Lifetime:            int64(m.Lifetime),
			ExpiresAt:           optional(m.ExpiresAt),
			Generation:          m.Generation,
			DeletionRequestedAt: optional(m.DeletionRequestedAt),
			CreatedAt:           millisecond(m.CreatedAt),
			UpdatedAt:           millisecond(m.UpdatedAt),
		},
		Spec:    spec,
		Status:  status,
		Version: record.Version,
		Control: control{
			Attempts: record.Attempts,
			TriedAt:  optional(record.TriedAt),
			Reset:    record.Reset,
			ResetAt:  optional(record.ResetAt),
			Restores: slices.Clone(record.Restores),
		},
	}

	for _, owner := range m.Owners {
		stored.Metadata.Owners = append(stored.Metadata.Owners, reference(owner))
	}

	for _, ref := range m.Refs {
		stored.Metadata.Refs = append(stored.Metadata.Refs, reference(ref))
	}

	if p := record.Pending; p != nil {
		payload, err := valueOf(p.Payload)
		if err != nil {
			return document{}, fmt.Errorf("the %s's pending %s cannot be stored: %w", record.Kind, p.Action, err)
		}

		stored.Control.Pending = &pending{Action: p.Action, Payload: payload, IDs: slices.Clone(p.IDs), SentAt: millisecond(p.SentAt)}
	}

	if a := record.Answer; a != nil {
		stored.Control.Answer = &answer{
			ID:      a.ID,
			Kind:    a.Kind,
			UUID:    a.UUID,
			Action:  a.Action,
			Node:    a.Node,
			Attempt: a.Attempt,
			OK:      a.OK,
			Refused: a.Refused,
			Reason:  a.Reason,
			Output:  a.Output,
			At:      millisecond(a.At),
		}
	}

	return stored, nil
}

func toRecord(stored document) (resource.Record, error) {
	spec, err := jsonOf(stored.Spec)
	if err != nil {
		return resource.Record{}, fmt.Errorf("the spec of %s %q cannot be read: %w", stored.Kind, stored.UUID, err)
	}

	status, err := jsonOf(stored.Status)
	if err != nil {
		return resource.Record{}, fmt.Errorf("the status of %s %q cannot be read: %w", stored.Kind, stored.UUID, err)
	}

	m := stored.Metadata

	record := resource.Record{
		Raw: kind.Raw{
			Kind: stored.Kind,
			Metadata: kind.Metadata{
				UUID:                stored.UUID,
				Name:                m.Name,
				Slug:                m.Slug,
				OwnerUUID:           m.OwnerUUID,
				Labels:              m.Labels,
				Node:                m.Node,
				Lifetime:            time.Duration(m.Lifetime),
				ExpiresAt:           moment(m.ExpiresAt),
				Generation:          m.Generation,
				DeletionRequestedAt: moment(m.DeletionRequestedAt),
				CreatedAt:           inUTC(m.CreatedAt),
				UpdatedAt:           inUTC(m.UpdatedAt),
			},
			Spec:   spec,
			Status: status,
		},
		Version:  stored.Version,
		Attempts: stored.Control.Attempts,
		TriedAt:  moment(stored.Control.TriedAt),
		Reset:    stored.Control.Reset,
		ResetAt:  moment(stored.Control.ResetAt),
		Restores: stored.Control.Restores,
	}

	for _, owner := range m.Owners {
		record.Metadata.Owners = append(record.Metadata.Owners, kind.Reference(owner))
	}

	for _, ref := range m.Refs {
		record.Metadata.Refs = append(record.Metadata.Refs, kind.Reference(ref))
	}

	if p := stored.Control.Pending; p != nil {
		payload, err := jsonOf(p.Payload)
		if err != nil {
			return resource.Record{}, fmt.Errorf("the pending %s of %s %q cannot be read: %w", p.Action, stored.Kind, stored.UUID, err)
		}

		record.Pending = &resource.Pending{Action: p.Action, Payload: payload, IDs: p.IDs, SentAt: inUTC(p.SentAt)}
	}

	if a := stored.Control.Answer; a != nil {
		record.Answer = &kind.ResourceActedOn{
			ID:      a.ID,
			Kind:    a.Kind,
			UUID:    a.UUID,
			Action:  a.Action,
			Node:    a.Node,
			Attempt: a.Attempt,
			OK:      a.OK,
			Refused: a.Refused,
			Reason:  a.Reason,
			Output:  a.Output,
			At:      inUTC(a.At),
		}
	}

	return record, nil
}

// valueOf is a kind's JSON as a value to store, as it is written: every
// object a document of the same keys in the same order, whatever they say,
// one given twice holding what it is given last, as JSON is read; every array
// an array; and every number as relaxed extended JSON reads one (number). No
// key is read as a type, as extended JSON reads $date or $numberLong: a label
// so named is a label like any other. Nothing at all is null. What cannot be
// kept as it is, a key with a NUL in it, or a number beyond what a double
// holds, is resource.ErrUnstorable.
func valueOf(raw json.RawMessage) (bson.RawValue, error) {
	if len(raw) == 0 {
		raw = json.RawMessage("null")
	}

	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()

	value, err := plain(decoder)
	if err == nil {
		if _, err = decoder.Token(); errors.Is(err, io.EOF) {
			err = nil
		} else if err == nil {
			err = errors.New("more follows the value")
		}
	}

	if err != nil {
		return bson.RawValue{}, fmt.Errorf("%w: %w", resource.ErrUnstorable, err)
	}

	wrapped, err := bson.Marshal(bson.D{{Key: "v", Value: value}})
	if err != nil {
		return bson.RawValue{}, fmt.Errorf("%w: %w", resource.ErrUnstorable, err)
	}

	return bson.Raw(wrapped).Lookup("v"), nil
}

// plain is the next value decoder reads, as valueOf stores it: a bson.D, a
// bson.A, a number as number has it, a string, a bool, or nil for null.
func plain(decoder *json.Decoder) (any, error) {
	token, err := decoder.Token()
	if err != nil {
		return nil, err
	}

	switch token := token.(type) {
	case json.Delim:
		if token == '[' {
			array := bson.A{}

			for decoder.More() {
				value, err := plain(decoder)
				if err != nil {
					return nil, err
				}

				array = append(array, value)
			}

			_, err := decoder.Token()

			return array, err
		}

		// an object, as nothing else opens a value.
		document := bson.D{}
		at := make(map[string]int)

		for decoder.More() {
			key, err := decoder.Token()
			if err != nil {
				return nil, err
			}

			value, err := plain(decoder)
			if err != nil {
				return nil, err
			}

			name, _ := key.(string)

			if i, given := at[name]; given {
				document[i].Value = value

				continue
			}

			at[name] = len(document)
			document = append(document, bson.E{Key: name, Value: value})
		}

		_, err := decoder.Token()

		return document, err
	case json.Number:
		return number(token)
	default:
		return token, nil
	}
}

// number is a JSON number as relaxed extended JSON reads one, which is how it
// is read back: one written with neither a fraction nor an exponent an int32,
// or an int64 beyond what that holds, and a double beyond both; and any other
// a double.
func number(written json.Number) (any, error) {
	if !strings.ContainsAny(written.String(), ".eE") {
		if integer, err := strconv.ParseInt(written.String(), 10, 64); err == nil {
			if integer < math.MinInt32 || integer > math.MaxInt32 {
				return integer, nil
			}

			return int32(integer), nil
		}
	}

	return strconv.ParseFloat(written.String(), 64)
}

// jsonOf is a stored value as the kind's JSON again. Null, or nothing stored,
// is nothing.
func jsonOf(value bson.RawValue) (json.RawMessage, error) {
	if value.Type == 0 || value.Type == bson.TypeNull {
		return nil, nil
	}

	wrapped, err := bson.Marshal(bson.D{{Key: "v", Value: value}})
	if err != nil {
		return nil, err
	}

	extended, err := bson.MarshalExtJSON(bson.Raw(wrapped), false, false)
	if err != nil {
		return nil, err
	}

	var unwrapped struct {
		V json.RawMessage `json:"v"`
	}

	if err := json.Unmarshal(extended, &unwrapped); err != nil {
		return nil, err
	}

	return unwrapped.V, nil
}

// optional is a time to store, or nothing for none.
func optional(t time.Time) *time.Time {
	if t.IsZero() {
		return nil
	}

	kept := millisecond(t)

	return &kept
}

// millisecond is a time as the database keeps it: to the millisecond, in UTC.
// A record comes back from a write as it would be read, times and all.
func millisecond(t time.Time) time.Time {
	if t.IsZero() {
		return time.Time{}
	}

	return t.UTC().Truncate(time.Millisecond)
}

// moment is a stored time, or none for nothing stored.
func moment(t *time.Time) time.Time {
	if t == nil {
		return time.Time{}
	}

	return inUTC(*t)
}

// inUTC is a time read back in UTC, which is what the database keeps, the
// zero time staying zero.
func inUTC(t time.Time) time.Time {
	if t.IsZero() {
		return time.Time{}
	}

	return t.UTC()
}
