package vm

import (
	"strconv"
	"strings"
	"time"
)

// A VM's restart policy is compose's, and vmhost applies it in place the way
// docker does: a task that ends is started again inside its machine, and a
// machine that goes away under its task is booted again from the same disks.
// Ported from PR #101's runner.

// The restart policies vmhost applies.
const (
	RestartNo            = "no"
	RestartAlways        = "always"
	RestartOnFailure     = "on-failure"
	RestartUnlessStopped = "unless-stopped"
)

// RestartPolicies are every policy vmhost applies, as a class offers them.
var RestartPolicies = []string{RestartNo, RestartAlways, RestartOnFailure, RestartUnlessStopped}

const (
	// the first wait before a task is started again, doubled each time it
	// ends again, up to the longest it is ever made to wait. It is what a
	// container runtime does, and for the same reason: a task that ends at
	// once is not started again as fast as it can end.
	firstBackoff   = 100 * time.Millisecond
	longestBackoff = time.Minute
)

// RestartPolicy is what becomes of a task that ends: no, always,
// unless-stopped, or on-failure with how many times at most (none is no
// limit).
type RestartPolicy struct {
	Name       string
	MaxRetries uint
}

// ParseRestartPolicy reads a policy as compose writes it. One it does not
// know restarts nothing, which is what naming none does.
func ParseRestartPolicy(policy string) RestartPolicy {
	name, retries, _ := strings.Cut(policy, ":")

	parsed := RestartPolicy{Name: name}
	if n, err := strconv.ParseUint(retries, 10, 32); err == nil {
		parsed.MaxRetries = uint(n)
	}

	return parsed
}

// IsRestartPolicy reports whether policy is one vmhost applies: empty, "no",
// "always", "unless-stopped", "on-failure", or "on-failure:N".
func IsRestartPolicy(policy string) bool {
	name, retries, counted := strings.Cut(policy, ":")

	switch name {
	case "", RestartNo, RestartAlways, RestartUnlessStopped:
		return !counted && (len(name) > 0 || len(policy) == 0)
	case RestartOnFailure:
		if !counted {
			return true
		}

		_, err := strconv.ParseUint(retries, 10, 32)

		return err == nil
	default:
		return false
	}
}

// Restarts reports whether a task that ended with exitCode, having been
// started again restarts times already, is started again. A task that was
// stopped on purpose never is: no policy undoes a stop, not even when vmhost
// itself is started again, since vmhost is redeployed with every release.
func (p RestartPolicy) Restarts(exitCode int, restarts uint, stopped bool) bool {
	if stopped {
		return false
	}

	switch p.Name {
	case RestartAlways, RestartUnlessStopped:
		return true
	case RestartOnFailure:
		return exitCode != 0 && (p.MaxRetries == 0 || restarts < p.MaxRetries)
	default:
		return false
	}
}

// Backoff is how long a task that has been started again restarts times is
// waited on before the next.
func Backoff(restarts uint) time.Duration {
	wait := firstBackoff

	for i := uint(0); i < restarts && wait < longestBackoff; i++ {
		wait *= 2
	}

	return min(wait, longestBackoff)
}
