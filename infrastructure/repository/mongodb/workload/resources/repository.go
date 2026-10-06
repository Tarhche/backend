// Package resources keeps the resources of every kind in MongoDB.
//
// Each kind has a collection of its own, named by its plural: a stack's are
// in stacks, a VM's in vms. A document is the resource's manifest, its
// metadata, spec and status, beside its version and what the control plane
// keeps of what it is in the middle of asking:
//
//	{_id: <uuid>, kind, metadata: {name, slug, owner_uuid, labels, owners,
//	 node, lifetime, expires_at, created_at, updated_at}, spec: {…},
//	 status: {…}, version, control: {pending, attempts, tried_at, answer,
//	 reset}}
//
// The spec and the status are the kind's own JSON, kept as documents of
// their own so that they can be read in the database as they are in the API.
// They are read from JSON as relaxed extended JSON, which every JSON a kind
// writes is, with two exceptions it is not expected to write: an object whose
// only key is one of extended JSON's own, such as $date, is read as what that
// key says; and an integer beyond what 64 bits hold is read as a double.
//
// # Indexes
//
// A kind's collection is indexed when the control plane starts, for every
// kind it registers (EnsureKind), rather than by a migration. The migrations
// are a fixed list, applied once each by `app migrate`, which runs without the
// control plane's registry and so cannot know which kinds there are; and a
// released migration is never edited, so it could not be told of a kind added
// later. Creating an index that is already there, with the same keys and
// options, changes nothing, so doing it at every start costs one round trip
// per kind, as the task logs' index already does. What a migration is still
// for is changing what is stored: a collection a kind takes over from the
// records kept before kinds, vms or stacks, is converted, and its old indexes
// dropped, by a migration of the step that takes it over.
package resources

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"slices"
	"sync"
	"time"

	"github.com/gofrs/uuid/v5"
	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"

	"github.com/khanzadimahdi/testproject/domain"
	"github.com/khanzadimahdi/testproject/domain/workload/kind"
	"github.com/khanzadimahdi/testproject/domain/workload/resource"
)

const queryTimeout = 3 * time.Second

// Repository keeps the resources of the kinds it was told of.
type Repository struct {
	database *mongo.Database

	lock        sync.RWMutex
	collections map[string]*mongo.Collection
}

var _ resource.Repository = &Repository{}

func NewRepository(database *mongo.Database) *Repository {
	if database == nil {
		panic("database should not be nil")
	}

	return &Repository{database: database, collections: make(map[string]*mongo.Collection)}
}

// EnsureKind makes the collection a kind's resources are kept in ready to
// keep them: named by the kind's plural, and indexed the ways resources are
// read. A kind it was not told of is not kept: asked for one, every method
// says kind.ErrUnknownKind.
//
// The indexes are a unique slug, among the resources that have one; a
// person's own, newest first; what one node holds, which every heartbeat
// reads; and what belongs to one resource, which is how a Docker VM's
// building blocks are listed.
func (r *Repository) EnsureKind(ctx context.Context, d kind.Descriptor) error {
	if len(d.Name) == 0 || len(d.Plural) == 0 {
		return fmt.Errorf("a kind is kept under its name and its plural, and %q has %q", d.Name, d.Plural)
	}

	collection := r.database.Collection(d.Plural)

	ctx, cancel := context.WithTimeout(ctx, queryTimeout)
	defer cancel()

	if _, err := collection.Indexes().CreateMany(ctx, []mongo.IndexModel{
		{Keys: bson.D{{Key: "metadata.slug", Value: 1}}, Options: options.Index().SetUnique(true).SetSparse(true)},
		{Keys: bson.D{{Key: "metadata.owner_uuid", Value: 1}, {Key: "_id", Value: -1}}},
		{Keys: bson.D{{Key: "metadata.node", Value: 1}}},
		{Keys: bson.D{{Key: "metadata.owners.uuid", Value: 1}, {Key: "metadata.owners.kind", Value: 1}}},
	}); err != nil {
		return fmt.Errorf("the %s cannot be indexed: %w", d.Plural, err)
	}

	r.lock.Lock()
	defer r.lock.Unlock()

	r.collections[d.Name] = collection

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

		return resource.Record{}, err
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
	// since is at another, and matches nothing.
	replaced, err := collection.ReplaceOne(ctx, bson.D{{Key: "_id", Value: stored.UUID}, {Key: "version", Value: read}}, stored)
	if mongo.IsDuplicateKeyError(err) {
		return resource.Record{}, domain.ErrAlreadyExists
	} else if err != nil {
		return resource.Record{}, err
	}

	if replaced.MatchedCount == 0 {
		exists, err := collection.CountDocuments(ctx, bson.D{{Key: "_id", Value: stored.UUID}})
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

	_, err = collection.DeleteOne(ctx, bson.D{{Key: "_id", Value: uuid}})

	return err
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

	query := filterOf(filter)

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

func (r *Repository) findOne(ctx context.Context, kindName string, filter bson.D) (resource.Record, error) {
	collection, err := r.collection(kindName)
	if err != nil {
		return resource.Record{}, err
	}

	ctx, cancel := context.WithTimeout(ctx, queryTimeout)
	defer cancel()

	var stored document
	if err := collection.FindOne(ctx, filter).Decode(&stored); err != nil {
		if errors.Is(err, mongo.ErrNoDocuments) {
			return resource.Record{}, domain.ErrNotExists
		}

		return resource.Record{}, err
	}

	return toRecord(stored)
}

// collection is the collection a kind's resources are kept in.
func (r *Repository) collection(kindName string) (*mongo.Collection, error) {
	r.lock.RLock()
	defer r.lock.RUnlock()

	collection, ok := r.collections[kindName]
	if !ok {
		return nil, fmt.Errorf("%w: no %q is kept here", kind.ErrUnknownKind, kindName)
	}

	return collection, nil
}

// filterOf is what a filter lets through, as a query.
func filterOf(filter resource.Filter) bson.D {
	query := bson.D{}

	if len(filter.OwnerUUID) > 0 {
		query = append(query, bson.E{Key: "metadata.owner_uuid", Value: filter.OwnerUUID})
	}

	if len(filter.Node) > 0 {
		query = append(query, bson.E{Key: "metadata.node", Value: filter.Node})
	}

	switch parent := filter.Parent; {
	case len(parent.UUID) == 0:
	case len(parent.Kind) == 0:
		query = append(query, bson.E{Key: "metadata.owners.uuid", Value: parent.UUID})
	default:
		query = append(query, bson.E{Key: "metadata.owners", Value: bson.D{{Key: "$elemMatch", Value: bson.D{
			{Key: "kind", Value: parent.Kind},
			{Key: "uuid", Value: parent.UUID},
		}}}})
	}

	// a label's key has dots in it, workload.flavor, which a path would take
	// for fields within fields, so each is read as the one field it is.
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
// which is sparse, holds only the resources reached under a name.
type metadata struct {
	Name      string            `bson:"name,omitempty"`
	Slug      string            `bson:"slug,omitempty"`
	OwnerUUID string            `bson:"owner_uuid,omitempty"`
	Labels    map[string]string `bson:"labels,omitempty"`
	Owners    []reference       `bson:"owners,omitempty"`
	Node      string            `bson:"node,omitempty"`
	Lifetime  int64             `bson:"lifetime,omitempty"`
	ExpiresAt *time.Time        `bson:"expires_at,omitempty"`
	CreatedAt time.Time         `bson:"created_at"`
	UpdatedAt time.Time         `bson:"updated_at"`
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
	Reason  string    `bson:"reason,omitempty"`
	Output  string    `bson:"output,omitempty"`
	At      time.Time `bson:"at"`
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
			Name:      m.Name,
			Slug:      m.Slug,
			OwnerUUID: m.OwnerUUID,
			Labels:    maps.Clone(m.Labels),
			Node:      m.Node,
			Lifetime:  int64(m.Lifetime),
			ExpiresAt: optional(m.ExpiresAt),
			CreatedAt: millisecond(m.CreatedAt),
			UpdatedAt: millisecond(m.UpdatedAt),
		},
		Spec:    spec,
		Status:  status,
		Version: record.Version,
		Control: control{
			Attempts: record.Attempts,
			TriedAt:  optional(record.TriedAt),
			Reset:    record.Reset,
		},
	}

	for _, owner := range m.Owners {
		stored.Metadata.Owners = append(stored.Metadata.Owners, reference(owner))
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
				UUID:      stored.UUID,
				Name:      m.Name,
				Slug:      m.Slug,
				OwnerUUID: m.OwnerUUID,
				Labels:    m.Labels,
				Node:      m.Node,
				Lifetime:  time.Duration(m.Lifetime),
				ExpiresAt: moment(m.ExpiresAt),
				CreatedAt: inUTC(m.CreatedAt),
				UpdatedAt: inUTC(m.UpdatedAt),
			},
			Spec:   spec,
			Status: status,
		},
		Version:  stored.Version,
		Attempts: stored.Control.Attempts,
		TriedAt:  moment(stored.Control.TriedAt),
		Reset:    stored.Control.Reset,
	}

	for _, owner := range m.Owners {
		record.Metadata.Owners = append(record.Metadata.Owners, kind.Reference(owner))
	}

	if p := stored.Control.Pending; p != nil {
		payload, err := jsonOf(p.Payload)
		if err != nil {
			return resource.Record{}, fmt.Errorf("the pending %s of %s %q cannot be read: %w", p.Action, stored.Kind, stored.UUID, err)
		}

		record.Pending = &resource.Pending{Action: p.Action, Payload: payload, IDs: p.IDs, SentAt: inUTC(p.SentAt)}
	}

	if a := stored.Control.Answer; a != nil {
		record.Answer = &kind.Result{
			ID:      a.ID,
			Kind:    a.Kind,
			UUID:    a.UUID,
			Action:  a.Action,
			Node:    a.Node,
			Attempt: a.Attempt,
			OK:      a.OK,
			Reason:  a.Reason,
			Output:  a.Output,
			At:      inUTC(a.At),
		}
	}

	return record, nil
}

// valueOf is a kind's JSON as a value to store. Nothing at all is null.
func valueOf(raw json.RawMessage) (bson.RawValue, error) {
	if len(raw) == 0 {
		raw = json.RawMessage("null")
	}

	wrapped := make([]byte, 0, len(raw)+6)
	wrapped = append(wrapped, `{"v":`...)
	wrapped = append(wrapped, raw...)
	wrapped = append(wrapped, '}')

	var value struct {
		V bson.RawValue `bson:"v"`
	}

	if err := bson.UnmarshalExtJSON(wrapped, false, &value); err != nil {
		return bson.RawValue{}, err
	}

	return value.V, nil
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
