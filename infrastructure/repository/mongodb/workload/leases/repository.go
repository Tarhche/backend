// Package leases keeps each root's lease in MongoDB, in workload_leases: one
// document for a root,
//
//	{_id: "<root uuid>", holder, expires_at, failures, reason, next_at,
//	 seen: {state, since}, poked, updated_at}
//
// holder and expires_at null while nobody holds it, next_at and seen while
// nothing backs its root off, and poked false but while another control plane
// asked for it while it was held. A lease kept before seen was has none,
// which is read as nothing seen.
//
// When a lease expires, and when a root's backoff is over, is judged by the
// database's clock, $$NOW, in the pipelines that write them, never by a
// control plane's: what each control plane writes is the time then, as the
// database has it, and as long after it as it asks for. So two control
// planes whose clocks disagree still agree on who holds a lease.
//
// A lease is taken with one conditional upsert of the root's document, by its
// _id alone: a pipeline that takes it when nobody holds it, it expired or its
// taker holds it already, and its root is not backed off, all as $$NOW says,
// leaves it as it was otherwise, and makes a new one when there is none. The
// condition is the pipeline's rather than the query's because MongoDB takes
// no $expr in an upsert's query, and the database's clock is read in an
// expression alone. What it is left as says what came of it: held by its
// taker, it was taken; held by another, it is held, and poked in the same
// write, so that its holder carries its root out again once it lets go of
// it; and held by nobody, its root is backed off, since a failure lets go of
// a lease as it backs its root off, so that a lease held is never backed
// off. Taken, it is poked no more. Two control planes that make a new one at
// once leave one of them a duplicate key, should MongoDB not take its upsert
// up again: the other made it, and holds it.
//
// A lease is taken now (TakeNow) with the same upsert, whose pipeline takes
// it whatever backs its root off, and clears that as it takes it: what a
// lease held by its taker says is that it took it, and one held by another
// that another holds it, so that of two that take one at once, one takes it.
//
// Only its holder renews it or lets go of it, which a write matched by its
// holder says, and a failure backs its root off for as long as the backoff
// it is given says, which the pipeline works out from the failures it
// counts, as graph.Backoff.After does, so that nothing is read before it is
// written.
//
// Nothing reads a lease but by its _id, which MongoDB indexes, so there is
// no index of its own to make: a root's plan, as the API shows it, reads its
// lease by its _id too (Get).
package leases

import (
	"context"
	"errors"
	"fmt"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"

	"github.com/khanzadimahdi/testproject/domain"
	"github.com/khanzadimahdi/testproject/domain/workload/graph"
	"github.com/khanzadimahdi/testproject/domain/workload/kind"
)

const (
	collectionName = "workload_leases"
	queryTimeout   = 3 * time.Second

	// now is the database's clock, as a pipeline reads it.
	now = "$$NOW"
)

// Repository keeps each root's lease.
type Repository struct {
	collection *mongo.Collection
}

var _ graph.Leases = &Repository{}

func NewRepository(database *mongo.Database) *Repository {
	if database == nil {
		panic("database should not be nil")
	}

	return &Repository{collection: database.Collection(collectionName)}
}

func (r *Repository) Take(ctx context.Context, rootUUID string, holder string, ttl time.Duration) (graph.Lease, error) {
	return r.take(ctx, rootUUID, holder, ttl, false)
}

func (r *Repository) TakeNow(ctx context.Context, rootUUID string, holder string, ttl time.Duration) (graph.Lease, error) {
	return r.take(ctx, rootUUID, holder, ttl, true)
}

// take takes the root's lease for holder, until ttl from now, as Take does,
// or, clearing, as TakeNow does: whatever backs its root off, which it
// clears.
func (r *Repository) take(ctx context.Context, rootUUID string, holder string, ttl time.Duration, clearing bool) (graph.Lease, error) {
	if err := asked(rootUUID, holder, ttl); err != nil {
		return graph.Lease{}, err
	}

	ctx, cancel := context.WithTimeout(ctx, queryTimeout)
	defer cancel()

	// taken when nobody holds it, it expired or holder holds it, and, but
	// when clearing, its root is not backed off: a null or missing time is
	// before $$NOW, and a null or missing holder is nobody, as a new one has.
	takeable := bson.D{{Key: "$or", Value: bson.A{
		bson.D{{Key: "$eq", Value: bson.A{bson.D{{Key: "$ifNull", Value: bson.A{"$holder", ""}}}, ""}}},
		bson.D{{Key: "$eq", Value: bson.A{"$holder", literal(holder)}}},
		bson.D{{Key: "$lte", Value: bson.A{"$expires_at", now}}},
	}}}

	if !clearing {
		takeable = bson.D{{Key: "$and", Value: bson.A{
			takeable,
			bson.D{{Key: "$not", Value: bson.A{bson.D{{Key: "$gt", Value: bson.A{"$next_at", now}}}}}},
		}}}
	}

	// held by another and not taken, it is poked, so that its holder carries
	// the root out again; poked before, it stays so.
	poked := bson.D{{Key: "$or", Value: bson.A{
		bson.D{{Key: "$ne", Value: bson.A{bson.D{{Key: "$ifNull", Value: bson.A{"$holder", ""}}}, ""}}},
		bson.D{{Key: "$ifNull", Value: bson.A{"$poked", false}}},
	}}}

	// what backs it off is kept as it was, and a new one has nothing backing
	// it off; taken clearing it, it is cleared.
	backedOff := func(field string, none any) bson.D {
		kept := bson.D{{Key: "$ifNull", Value: bson.A{"$" + field, none}}}
		if clearing {
			return ifTaken(none, kept)
		}

		return kept
	}

	// what is taken is held by holder from now, and poked no more, and what
	// is not is left as it was but for being poked.
	take := mongo.Pipeline{
		{{Key: "$set", Value: bson.D{{Key: "taken", Value: takeable}}}},
		{{Key: "$set", Value: bson.D{
			{Key: "holder", Value: ifTaken(literal(holder), "$holder")},
			{Key: "expires_at", Value: ifTaken(after(ttl), "$expires_at")},
			{Key: "failures", Value: backedOff("failures", 0)},
			{Key: "reason", Value: backedOff("reason", "")},
			{Key: "next_at", Value: backedOff("next_at", nil)},
			{Key: "seen", Value: backedOff("seen", nil)},
			{Key: "poked", Value: ifTaken(false, poked)},
			{Key: "updated_at", Value: ifTaken(now, "$updated_at")},
		}}},
		{{Key: "$unset", Value: "taken"}},
	}

	var left document
	err := r.collection.FindOneAndUpdate(
		ctx,
		bson.D{{Key: "_id", Value: rootUUID}},
		take,
		options.FindOneAndUpdate().SetUpsert(true).SetReturnDocument(options.After),
	).Decode(&left)

	switch {
	case mongo.IsDuplicateKeyError(err):
		return graph.Lease{Root: rootUUID}, graph.ErrHeld
	case err != nil:
		return graph.Lease{}, err
	}

	lease := toLease(left)

	switch lease.Holder {
	case holder:
		return lease, nil
	case "":
		return lease, graph.ErrBackedOff
	}

	return lease, graph.ErrHeld
}

func (r *Repository) Renew(ctx context.Context, rootUUID string, holder string, ttl time.Duration) (graph.Lease, error) {
	if err := asked(rootUUID, holder, ttl); err != nil {
		return graph.Lease{}, err
	}

	return r.held(ctx, rootUUID, holder, mongo.Pipeline{{{Key: "$set", Value: bson.D{
		{Key: "expires_at", Value: after(ttl)},
		{Key: "updated_at", Value: now},
	}}}})
}

func (r *Repository) Release(ctx context.Context, rootUUID string, holder string, ended graph.Ended) (graph.Lease, error) {
	if err := asked(rootUUID, holder, time.Millisecond); err != nil {
		return graph.Lease{}, err
	}

	released := bson.D{
		{Key: "holder", Value: nil},
		{Key: "expires_at", Value: nil},
		{Key: "updated_at", Value: now},
	}

	var release mongo.Pipeline

	switch ended.State {
	case graph.StateDone:
		release = mongo.Pipeline{{{Key: "$set", Value: append(released,
			bson.E{Key: "failures", Value: 0},
			bson.E{Key: "reason", Value: ""},
			bson.E{Key: "next_at", Value: nil},
			bson.E{Key: "seen", Value: nil},
		)}}}
	case graph.StateWaiting:
		release = mongo.Pipeline{{{Key: "$set", Value: released}}}
	case graph.StateFailed:
		// one more failure first, and then the backoff so many earn.
		release = mongo.Pipeline{
			{{Key: "$set", Value: append(released,
				bson.E{Key: "failures", Value: bson.D{{Key: "$add", Value: bson.A{bson.D{{Key: "$ifNull", Value: bson.A{"$failures", 0}}}, 1}}}},
				bson.E{Key: "reason", Value: literal(ended.Reason)},
				bson.E{Key: "seen", Value: seenOf(ended.Seen)},
			)}},
			{{Key: "$set", Value: bson.D{
				{Key: "next_at", Value: bson.D{{Key: "$add", Value: bson.A{now, backoff(ended.Backoff)}}}},
			}}},
		}
	default:
		return graph.Lease{}, fmt.Errorf("a run ends done, waiting or failed, not %q", ended.State)
	}

	return r.held(ctx, rootUUID, holder, release)
}

func (r *Repository) Forget(ctx context.Context, rootUUID string) error {
	ctx, cancel := context.WithTimeout(ctx, queryTimeout)
	defer cancel()

	_, err := r.collection.DeleteOne(ctx, bson.D{{Key: "_id", Value: rootUUID}})

	return err
}

func (r *Repository) Get(ctx context.Context, rootUUID string) (graph.Lease, error) {
	ctx, cancel := context.WithTimeout(ctx, queryTimeout)
	defer cancel()

	var stored document
	err := r.collection.FindOne(ctx, bson.D{{Key: "_id", Value: rootUUID}}).Decode(&stored)

	switch {
	case errors.Is(err, mongo.ErrNoDocuments):
		return graph.Lease{}, domain.ErrNotExists
	case err != nil:
		return graph.Lease{}, err
	}

	return toLease(stored), nil
}

// held writes the root's lease, which holder holds, as change says, and is
// the lease as it is then: ErrLost when holder does not hold it.
func (r *Repository) held(ctx context.Context, rootUUID string, holder string, change mongo.Pipeline) (graph.Lease, error) {
	ctx, cancel := context.WithTimeout(ctx, queryTimeout)
	defer cancel()

	var written document
	err := r.collection.FindOneAndUpdate(
		ctx,
		bson.D{{Key: "_id", Value: rootUUID}, {Key: "holder", Value: holder}},
		change,
		options.FindOneAndUpdate().SetReturnDocument(options.After),
	).Decode(&written)

	switch {
	case errors.Is(err, mongo.ErrNoDocuments):
		return graph.Lease{}, graph.ErrLost
	case err != nil:
		return graph.Lease{}, err
	}

	return toLease(written), nil
}

// document is a lease as it is stored: every field there, null while it has
// none.
type document struct {
	Root      string        `bson:"_id"`
	Holder    string        `bson:"holder"`
	ExpiresAt *time.Time    `bson:"expires_at"`
	Failures  int           `bson:"failures"`
	Reason    string        `bson:"reason"`
	NextAt    *time.Time    `bson:"next_at"`
	Seen      *seenDocument `bson:"seen"`
	Poked     bool          `bson:"poked"`
	UpdatedAt *time.Time    `bson:"updated_at"`
}

// seenDocument is what the run whose failure backs a lease's root off saw of
// the root, as it is stored.
type seenDocument struct {
	State string     `bson:"state"`
	Since *time.Time `bson:"since"`
}

func toLease(stored document) graph.Lease {
	lease := graph.Lease{
		Root:      stored.Root,
		Holder:    stored.Holder,
		ExpiresAt: moment(stored.ExpiresAt),
		Failures:  stored.Failures,
		Reason:    stored.Reason,
		NextAt:    moment(stored.NextAt),
		Poked:     stored.Poked,
		UpdatedAt: moment(stored.UpdatedAt),
	}

	if stored.Seen != nil {
		lease.Seen = graph.Seen{State: kind.State(stored.Seen.State), Since: moment(stored.Seen.Since)}
	}

	return lease
}

// seenOf is what a run saw as it is stored, taken as it is: its time to the
// millisecond, which is all a date of the database's holds, and null when it
// saw nothing.
func seenOf(s graph.Seen) any {
	if len(s.State) == 0 {
		return nil
	}

	var since any
	if !s.Since.IsZero() {
		since = s.Since.UTC().Truncate(time.Millisecond)
	}

	return literal(bson.D{{Key: "state", Value: string(s.State)}, {Key: "since", Value: since}})
}

// after is the time d from now, by the database's clock, to the millisecond.
func after(d time.Duration) bson.D {
	return bson.D{{Key: "$add", Value: bson.A{now, d.Milliseconds()}}}
}

// backoff is how long, in milliseconds, a root is backed off after as many
// failures in a row as its lease counts, as b.After says it: b's first
// doubled once for each failure in a row but the first, as many times as
// graph.MaxDoublings at most, and never more than b's most.
func backoff(b graph.Backoff) bson.D {
	doublings := bson.D{{Key: "$min", Value: bson.A{
		bson.D{{Key: "$max", Value: bson.A{bson.D{{Key: "$subtract", Value: bson.A{"$failures", 1}}}, 0}}},
		graph.MaxDoublings,
	}}}

	return bson.D{{Key: "$min", Value: bson.A{
		b.Most.Milliseconds(),
		bson.D{{Key: "$multiply", Value: bson.A{
			b.First.Milliseconds(),
			bson.D{{Key: "$pow", Value: bson.A{2, doublings}}},
		}}},
	}}}
}

// ifTaken is taken when the lease a pipeline takes is, and kept otherwise.
func ifTaken(taken any, kept any) bson.D {
	return bson.D{{Key: "$cond", Value: bson.A{"$taken", taken, kept}}}
}

// literal is a value a pipeline takes as it is, never as a field's path or
// an operator, whatever it starts with.
func literal(value any) bson.D {
	return bson.D{{Key: "$literal", Value: value}}
}

// moment is a stored time, in UTC, or none for nothing stored.
func moment(t *time.Time) time.Time {
	if t == nil || t.IsZero() {
		return time.Time{}
	}

	return t.UTC()
}

// asked is what is wrong with a lease of the root uuid names, for holder,
// for ttl: nothing, unless one of them is nothing, or ttl is shorter than a
// millisecond.
func asked(rootUUID string, holder string, ttl time.Duration) error {
	switch {
	case len(rootUUID) == 0:
		return errors.New("a lease is a root's, by its uuid, and this one names none")
	case len(holder) == 0:
		return errors.New("a lease is held by a control plane, and this one names none")
	case ttl < time.Millisecond:
		return fmt.Errorf("a lease is held for a millisecond at least, not %s", ttl)
	}

	return nil
}
