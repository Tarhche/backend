package task

const (
	// RetryForever asks the workload never to give up on a task: however
	// many times it fails, it is asked for again.
	RetryForever = -1

	// serviceRetries is how many times a service is tried again when nothing
	// says otherwise. A task that has failed this many times in a row is
	// failing for a reason that asking again does not fix.
	serviceRetries = 3

	// jobRetries is how many times a job is tried again, which is none: a job
	// is asked for once, by somebody waiting for its output, and running it
	// twice would give them the wrong one.
	jobRetries = 0
)

// DefaultMaxRetries is what a task gets when it did not say how many
// times it is worth trying.
func DefaultMaxRetries(kind Kind) int {
	if kind == KindJob {
		return jobRetries
	}

	return serviceRetries
}
