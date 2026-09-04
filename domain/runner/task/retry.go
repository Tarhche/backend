package task

import "time"

const (
	// RetryForever asks the runner never to give up on a container: however
	// many times it fails, it is asked for again.
	RetryForever = -1

	// serviceRetries is how many times a service is tried again when nothing
	// says otherwise. A container that has failed this many times in a row is
	// failing for a reason that asking again does not fix.
	serviceRetries = 3

	// jobRetries is how many times a job is tried again, which is none: a job
	// is asked for once, by somebody waiting for its output, and running it
	// twice would give them the wrong one.
	jobRetries = 0

	// RetryWindow is how long a container has to stand up before the failures
	// behind it stop counting. A container that ran for an afternoon and then
	// died is not the container that could not start this morning.
	RetryWindow = 5 * time.Minute

	// retryDelayStep is how long the runner leaves a container that has failed
	// once, and how much longer for each failure behind that one;
	// retryDelayCeiling is as long as it ever leaves one.
	retryDelayStep    = 2 * time.Second
	retryDelayCeiling = 30 * time.Second
)

// RetryDelay is how long to leave a container that has failed its attempt-th
// attempt before making the next one.
//
// Even the first wait is a wait: a container that fails the moment it starts
// would otherwise go through everything it is worth faster than anybody
// watching could see what was happening to it.
func RetryDelay(attempt int) time.Duration {
	if attempt < 0 {
		attempt = 0
	}

	delay := time.Duration(attempt+1) * retryDelayStep
	if delay > retryDelayCeiling {
		return retryDelayCeiling
	}

	return delay
}

// DefaultMaxRetries is what a container gets when it did not say how many
// times it is worth trying.
func DefaultMaxRetries(kind Kind) int {
	if kind == KindJob {
		return jobRetries
	}

	return serviceRetries
}

// MayRetry reports whether a container that has just failed its attempt-th
// attempt is worth another one.
//
// Attempts are counted in the messages that carry them rather than written
// down: a count belongs to one run of failures, and there is nothing left of it
// once the container is what it was asked to be.
func (t *Task) MayRetry(attempt int) bool {
	if t.MaxRetries == RetryForever {
		return true
	}

	return attempt < t.MaxRetries
}

// RetryDue reports whether a container that has failed has been left long
// enough to be worth another attempt.
//
// The wait grows with the attempts behind it, so a container that fails the
// moment it starts is not started over and over as fast as it can fail. It is
// measured from the failure itself, which is written down, so the wait survives
// a manager that is restarted in the middle of it.
func (t *Task) RetryDue(now time.Time, attempt int) bool {
	if t.ExpectedState != Running || t.FinishedAt.IsZero() {
		return false
	}

	return now.Sub(t.FinishedAt) >= RetryDelay(attempt)
}

// Attempt is which attempt a failure belongs to, given the number the node
// reported with it.
//
// A container that had been standing for a while before it failed starts the
// count again: what it failed at is staying up, not coming up, and the attempts
// it took to come up were another story.
func (t *Task) Attempt(reported int, failedAt time.Time) int {
	if t.StartedAt.IsZero() || failedAt.IsZero() || failedAt.Sub(t.StartedAt) < RetryWindow {
		return reported
	}

	return 0
}
