package firecracker

import (
	"strconv"
	"strings"
	"time"
)

const (
	// the first wait before a task is started again, doubled each time it
	// ends again, up to the longest it is ever made to wait. It is what a
	// container runtime does, and for the same reason: a task that ends at
	// once is not started again as fast as it can end.
	firstBackoff   = 100 * time.Millisecond
	longestBackoff = time.Minute
)

// restartPolicy is what becomes of a task that ends, as a compose file says
// it: no, always, unless-stopped, or on-failure with how many times.
type restartPolicy struct {
	name       string
	maxRetries uint
}

func parsePolicy(policy string) restartPolicy {
	name, retries, _ := strings.Cut(policy, ":")

	parsed := restartPolicy{name: name}
	if n, err := strconv.ParseUint(retries, 10, 32); err == nil {
		parsed.maxRetries = uint(n)
	}

	return parsed
}

// restarts reports whether a task that ended with exitCode, having been
// started again restarts times already, is started again. A task that was
// stopped on purpose never is.
func (p restartPolicy) restarts(exitCode int, restarts uint, stopped bool) bool {
	if stopped {
		return false
	}

	switch p.name {
	case "always", "unless-stopped":
		return true
	case "on-failure":
		return exitCode != 0 && (p.maxRetries == 0 || restarts < p.maxRetries)
	default:
		return false
	}
}

// backoff is how long a task that has been started again restarts times is
// waited on before the next.
func backoff(restarts uint) time.Duration {
	wait := firstBackoff

	for i := uint(0); i < restarts && wait < longestBackoff; i++ {
		wait *= 2
	}

	return min(wait, longestBackoff)
}
