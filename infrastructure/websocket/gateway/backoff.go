package gateway

import "time"

const (
	defaultReplyAttempts = 3
	defaultReplyWait     = time.Second

	defaultQueueAttempts = 3
	defaultQueueWait     = 50 * time.Millisecond
)

// Backoff decides whether a reply whose routing failed is tried again, and how
// long to wait first.
//
// One Backoff is shared by every connection, so Next is called concurrently and
// an implementation must be safe for concurrent use. The ones here hold no
// state; anything that counts, jitters or adapts needs its own synchronisation.
type Backoff interface {
	// Next reports the wait before the given attempt, which is 1-based, and
	// whether that attempt should be made at all.
	Next(attempt int) (time.Duration, bool)
}

// fixedBackoff waits the same amount before every attempt. It is an immutable
// value, so sharing one across connections is safe.
type fixedBackoff struct {
	attempts int
	wait     time.Duration
}

// NewFixedBackoff retries up to attempts times, waiting the same amount before
// each one.
func NewFixedBackoff(attempts int, wait time.Duration) Backoff {
	return fixedBackoff{
		attempts: attempts,
		wait:     wait,
	}
}

func (b fixedBackoff) Next(attempt int) (time.Duration, bool) {
	if attempt >= b.attempts {
		return 0, false
	}

	return b.wait, true
}
