package runs

import (
	"fmt"
	"strconv"
	"strings"
	"time"
)

// The restart policies, as docker names them.
const (
	policyNo            = "no"
	policyAlways        = "always"
	policyOnFailure     = "on-failure"
	policyUnlessStopped = "unless-stopped"
)

// policy is a run's restart policy. Microsandbox has none of its own, so the
// supervisor applies docker's, to the letter where it can.
type policy struct {
	name string

	// maxRetries bounds on-failure's restarts; zero is no bound.
	maxRetries uint
}

// parsePolicy reads a restart policy the way docker does: "no", "always",
// "unless-stopped", "on-failure" or "on-failure:N", and empty for "no". Only
// on-failure takes a bound.
func parsePolicy(value string) (policy, error) {
	name, bound, bounded := strings.Cut(value, ":")

	switch name {
	case "", policyNo:
		name = policyNo
	case policyAlways, policyUnlessStopped, policyOnFailure:
	default:
		return policy{}, fmt.Errorf("restart policy %q is not one of no, always, on-failure[:N] and unless-stopped", value)
	}

	if !bounded {
		return policy{name: name}, nil
	}

	if name != policyOnFailure {
		return policy{}, fmt.Errorf("restart policy %q takes no bound: only on-failure does", value)
	}

	retries, err := strconv.ParseUint(bound, 10, 32)
	if err != nil {
		return policy{}, fmt.Errorf("restart policy %q has to bound on-failure by a whole number of retries", value)
	}

	return policy{name: name, maxRetries: uint(retries)}, nil
}

// restarts is whether a main process that ended by itself with exitCode is
// started again, its run having been restarted restartCount times by the
// policy already.
func (p policy) restarts(exitCode int, restartCount uint) bool {
	switch p.name {
	case policyAlways, policyUnlessStopped:
		return true
	case policyOnFailure:
		return exitCode != 0 && (p.maxRetries == 0 || restartCount < p.maxRetries)
	default:
		return false
	}
}

// revives is whether a run that was running when the service went away is
// started again when it comes back.
//
// Docker brings back always and unless-stopped containers when its daemon
// restarts, and leaves on-failure and no ones exited: they did not fail, the
// daemon did. Microsandbox runs follow it, and the control plane brings back
// the rest as it brings back a task whose node went away.
func (p policy) revives() bool {
	return p.name == policyAlways || p.name == policyUnlessStopped
}

// next is how long the restart after a run that stayed up for ranFor waits,
// the one before it having waited previous: docker's backoff, which doubles
// from Initial up to Max and starts over for a run that stayed up for
// ResetAfter, so that a run that crashes now and then is not held back by one
// that crashed often long ago.
func (b Backoff) next(previous time.Duration, ranFor time.Duration) time.Duration {
	if ranFor >= b.ResetAfter {
		previous = 0
	}

	wait := previous * 2
	if previous == 0 {
		wait = b.Initial
	}

	return min(wait, b.Max)
}
