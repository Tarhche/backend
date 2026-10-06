package kindstest

import (
	"context"
	"sync"
	"time"

	"github.com/khanzadimahdi/testproject/domain/workload/kind"
	"github.com/khanzadimahdi/testproject/domain/workload/resource"
)

const (
	// NodeName is the node fans are held on, unless a test says otherwise.
	NodeName = "node-1"

	// House is the house fans are in, and OwnerUUID whom they belong to.
	House     = "house-1"
	OwnerUUID = "owner-uuid"
)

// Moment is when a test happens.
var Moment = time.Date(2026, 10, 6, 12, 0, 0, 0, time.UTC)

// Clock is a time a test moves on by hand.
type Clock struct {
	lock sync.Mutex
	now  time.Time
}

// NewClock is a clock that says Moment.
func NewClock() *Clock {
	return &Clock{now: Moment}
}

// Now is the time on the clock.
func (c *Clock) Now() time.Time {
	c.lock.Lock()
	defer c.lock.Unlock()

	return c.now
}

// Advance moves the clock on.
func (c *Clock) Advance(d time.Duration) {
	c.lock.Lock()
	defer c.lock.Unlock()

	c.now = c.now.Add(d)
}

// AFan is a fan's record: the uuid's, in its house, on NodeName, doing state
// since Moment and expected to be expected, as changes say otherwise.
func AFan(uuid string, state kind.State, expected kind.State, changes ...func(*Fan)) resource.Record {
	fan := Fan{
		Kind: Kind,
		Metadata: kind.Metadata{
			UUID:      uuid,
			Name:      "kitchen",
			Slug:      "kitchen-" + uuid,
			OwnerUUID: OwnerUUID,
			Owners:    []kind.Reference{{Kind: Parent, UUID: House}},
			Node:      NodeName,
			CreatedAt: Moment,
			UpdatedAt: Moment,
		},
		Spec:   Spec{Blades: 3, House: House},
		Status: Status{Status: kind.Status{State: state, Expected: expected, Since: Moment}, Speed: 1},
	}

	for _, change := range changes {
		change(&fan)
	}

	raw, err := kind.Encode(fan)
	if err != nil {
		panic(err)
	}

	return resource.Record{Raw: raw}
}

// Typed is a record read as a fan.
func Typed(r resource.Record) Fan {
	fan, err := kind.Decode[Spec, Status](r.Raw)
	if err != nil {
		panic(err)
	}

	return fan
}

// Racing is a repository in which something else writes a resource just
// before it is next written, so that what was read before is stale by then:
// a heartbeat crossing a result, a person crossing the reconcile loop.
type Racing struct {
	resource.Repository

	lock   sync.Mutex
	before []func(ctx context.Context, r resource.Record)
}

var _ resource.Repository = &Racing{}

// Cross has write happen just before each of the next updates, one each, in
// order.
func (r *Racing) Cross(write ...func(ctx context.Context, r resource.Record)) {
	r.lock.Lock()
	defer r.lock.Unlock()

	r.before = append(r.before, write...)
}

func (r *Racing) Update(ctx context.Context, record resource.Record) (resource.Record, error) {
	r.lock.Lock()
	var crossing func(ctx context.Context, r resource.Record)
	if len(r.before) > 0 {
		crossing, r.before = r.before[0], r.before[1:]
	}
	r.lock.Unlock()

	if crossing != nil {
		crossing(ctx, record)
	}

	return r.Repository.Update(ctx, record)
}

// Rewrite is a crossing write that changes a resource as change says,
// writing over whatever version is stored.
func Rewrite(repository resource.Repository, change func(*resource.Record)) func(ctx context.Context, r resource.Record) {
	return func(ctx context.Context, r resource.Record) {
		stored, err := repository.GetOne(ctx, r.Kind, r.Metadata.UUID)
		if err != nil {
			panic(err)
		}

		change(&stored)

		if _, err := repository.Update(ctx, stored); err != nil {
			panic(err)
		}
	}
}
