package graph

import (
	"context"
	"errors"
	"time"
)

var (
	// ErrHeld is a root's lease another control plane holds and has not let
	// expire: it is carrying the root's plan out.
	ErrHeld = errors.New("the root's lease is held by another control plane")

	// ErrBackedOff is a root's lease asked for before the backoff the last
	// failure of its plan earned it is over (Lease.NextAt): it is carried
	// out again once it is.
	ErrBackedOff = errors.New("the root is backed off after its plan failed")

	// ErrLost is a root's lease renewed or released by what no longer holds
	// it: it expired and another control plane took it, or it was forgotten.
	ErrLost = errors.New("the root's lease is not held by this control plane")
)

// MaxDoublings is how many times a backoff is doubled at most, however many
// failures in a row it follows: First doubled forty times is past any Most a
// control plane is configured with, a millisecond's being thirty-four years.
// A repository that works a backoff out itself doubles it as many times at
// most, so that it is what After says.
const MaxDoublings = 40

// Lease is one root's: which control plane carries the root's plan out, and
// until when, and how its plan went lately, which backs the root off once it
// fails.
type Lease struct {
	// Root is the uuid of the root it is of.
	Root string

	// Holder is the control plane that holds it, and ExpiresAt when another
	// may take it, unless its holder renews it first: none while nobody
	// holds it.
	Holder    string
	ExpiresAt time.Time

	// Failures are how many runs of the root's plan in a row failed, Reason
	// what the last of them failed with, and NextAt when the root may be
	// carried out again: none since a run was last done.
	Failures int
	Reason   string
	NextAt   time.Time

	// UpdatedAt is when it was written last.
	UpdatedAt time.Time
}

// Backoff is how long a root whose plan failed is left before it is carried
// out again: First after its first failure, and twice as long after each
// failure in a row since, up to Most.
type Backoff struct {
	First time.Duration
	Most  time.Duration
}

// After is how long a root is left after failures runs of its plan in a row
// failed: nothing before any has.
func (b Backoff) After(failures int) time.Duration {
	if failures <= 0 {
		return 0
	}

	wait := b.First
	for range min(failures-1, MaxDoublings) {
		if wait >= b.Most {
			break
		}

		wait *= 2
	}

	return min(wait, b.Most)
}

// Ended is how a run of a root's plan ended, which its lease keeps once the
// run lets it go (Leases.Release).
type Ended struct {
	// State is what the run came to: StateDone, which clears what backed
	// the root off; StateWaiting, which leaves it as it was; or StateFailed,
	// one more failure in a row, whose Reason is what it failed with, which
	// backs the root off for as long as Backoff says of so many.
	State   State
	Reason  string
	Backoff Backoff
}

// Leases keeps each root's lease, by the root's uuid: what keeps two control
// planes from carrying one root's plan out at once, and backs a root off once
// its plan fails. When a lease expires, and when a root's backoff is over, is
// judged by one clock, the one that keeps the leases, so that the control
// planes' own do not matter.
type Leases interface {
	// Take takes the root's lease for holder, until ttl from now, and is the
	// lease as it is then: one nobody holds, one that expired and one holder
	// holds already is taken, its failures, and when the root may be carried
	// out next, kept as they were. One whose root is backed off is not
	// taken, whoever holds it, and is ErrBackedOff; one another holds, and
	// has not let expire, is ErrHeld; and either is beside the lease as it
	// is. A root's uuid or a holder that is nothing is an error, and so is a
	// ttl shorter than a millisecond, and nothing is taken.
	Take(ctx context.Context, rootUUID string, holder string, ttl time.Duration) (Lease, error)

	// Renew has the root's lease, which holder holds, expire ttl from now,
	// and is the lease as it is then. One holder does not hold is ErrLost,
	// and is not renewed; and what Take refuses to take a lease for, it
	// refuses to renew one for.
	Renew(ctx context.Context, rootUUID string, holder string, ttl time.Duration) (Lease, error)

	// Release lets go of the root's lease, which holder holds, keeping how
	// the run that held it ended, and is the lease as it is left: a run
	// done clears its failures, a run that waits leaves them as they were,
	// and a run that failed is one more, which backs the root off from now,
	// for as long as its Backoff says of so many in a row. One holder does
	// not hold is ErrLost, an end that is none of those three, and a root's
	// uuid that is nothing, are errors, and none of them changes anything.
	Release(ctx context.Context, rootUUID string, holder string, ended Ended) (Lease, error)

	// Forget removes the root's lease, its failures with it, once the root
	// is gone. One there is none of is forgotten already.
	Forget(ctx context.Context, rootUUID string) error

	// Get is the root's lease as it is kept, held or not, taking nothing:
	// how its plan went lately, for whoever asks. One there is none of is
	// domain.ErrNotExists.
	Get(ctx context.Context, rootUUID string) (Lease, error)
}
